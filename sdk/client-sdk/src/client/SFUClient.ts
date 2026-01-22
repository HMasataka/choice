/**
 * Main SFU Client class
 */

import { EventEmitter } from '../events/EventEmitter';
import { SignalingClient } from '../signaling/SignalingClient';
import { PeerConnection } from '../webrtc/PeerConnection';
import { Room } from './Room';
import { LocalParticipant, RemoteParticipant } from './Participant';
import { SFUError, ErrorCodes } from '../errors/SFUError';
import { Logger } from '../utils/logger';
import { retry, DEFAULT_RECONNECT_CONFIG } from '../utils/retry';
import type {
  SFUClientConfig,
  JoinOptions,
  ConnectionState,
  DisconnectReason,
  ReconnectConfig,
  PublishOptions,
  SubscribeOptions,
  SimulcastLayer,
} from '../utils/types';
import type { ParticipantData } from '../signaling/types';

/** Client events */
interface ClientEvents {
  connecting: void;
  connected: void;
  disconnected: DisconnectReason;
  reconnecting: void;
  reconnected: void;
  error: SFUError;
}

/**
 * SFU Client - main entry point for the SDK
 */
export class SFUClient extends EventEmitter<ClientEvents> {
  private readonly config: Required<
    Pick<SFUClientConfig, 'url' | 'autoReconnect'>
  > & {
    reconnect: ReconnectConfig;
    iceServers: RTCIceServer[];
  };
  private readonly signaling: SignalingClient;
  private readonly logger: Logger;

  private peerConnection: PeerConnection | null = null;
  private room: Room | null = null;
  private connectionState: ConnectionState = 'disconnected';
  private reconnectAttempt = 0;
  private isReconnecting = false;
  /** Stored token for reconnection */
  private lastToken: string | null = null;
  /** Map of pending subscriptions: trackId -> subscriptionId (for track matching) */
  private pendingSubscriptions: Map<string, string> = new Map();

  constructor(config: SFUClientConfig) {
    super();

    this.config = {
      url: config.url,
      autoReconnect: config.autoReconnect ?? true,
      reconnect: { ...DEFAULT_RECONNECT_CONFIG, ...config.reconnect },
      iceServers: config.iceServers ?? [],
    };

    this.logger = new Logger(config.logger, 'Client');
    this.signaling = new SignalingClient({
      logger: config.logger,
    });

    this.setupSignalingHandlers();
  }

  /**
   * Set up signaling event handlers
   */
  private setupSignalingHandlers(): void {
    this.signaling.on('connected', () => {
      this.setConnectionState('connected');
      this.emit('connected', undefined);
    });

    this.signaling.on('disconnected', (event) => {
      this.handleDisconnect(event);
    });

    this.signaling.on('error', () => {
      this.emit('error', new SFUError(ErrorCodes.SIGNALING_ERROR, 'Signaling error'));
    });

    // Room notifications
    this.signaling.on('participantJoined', (data) => {
      if (this.room !== null) {
        const participant = this.createRemoteParticipant(data.participant);
        this.room.addParticipant(participant);
      }
    });

    this.signaling.on('participantLeft', (data) => {
      if (this.room !== null) {
        this.room.removeParticipant(data.participantId, data.reason);
      }
    });

    this.signaling.on('trackPublished', (data) => {
      if (this.room !== null) {
        this.room.handleTrackPublished(
          data.publisherId,
          data.track.id,
          data.track.kind,
          data.track.simulcast,
          data.track.metadata,
          data.track.name
        );
      }
    });

    this.signaling.on('trackUnpublished', (data) => {
      if (this.room !== null) {
        this.room.handleTrackUnpublished(data.publisherId, data.trackId);
      }
    });

    this.signaling.on('trackSubscribed', (data) => {
      this.logger.debug('Track subscribed notification', data);
      // Set subscription ID on RemoteTrack for more reliable matching
      if (this.room !== null) {
        this.room.handleTrackSubscriptionConfirmed(
          data.publisherId,
          data.trackId,
          data.subscriptionId
        );
      }
      // Clear pending subscription now that track has subscriptionId set
      // This prevents stale entries if ontrack never arrives
      this.pendingSubscriptions.delete(data.trackId);
    });

    this.signaling.on('trackSubscriptionFailed', (data) => {
      // Clean up pending subscription on failure
      this.pendingSubscriptions.delete(data.trackId);

      if (this.room !== null) {
        this.room.handleTrackSubscriptionFailed(
          data.trackId,
          SFUError.fromJsonRpcError(data.error)
        );
      }
    });

    this.signaling.on('layerChanged', (data) => {
      if (this.room !== null) {
        this.room.handleLayerChanged({
          trackId: data.trackId,
          requestedLayer: data.requestedLayer,
          actualLayer: data.actualLayer,
          reason: data.reason,
        });
      }
    });

    this.signaling.on('connectionQualityChanged', (data) => {
      if (this.room !== null) {
        this.room.handleConnectionQualityChanged(data.participantId, data.quality);
      }
    });

    this.signaling.on('serverStateChanged', (data) => {
      if (this.room !== null) {
        this.room.updateServerState(data.state);
      }
    });

    this.signaling.on('serverError', (data) => {
      if (this.room !== null) {
        this.room.handleServerError(data);
      }
      if (data.fatal) {
        this.emit('error', new SFUError(data.code, data.message));
      }
    });

    this.signaling.on('reconnectRequested', (data) => {
      if (this.room !== null) {
        this.room.handleReconnectRequested(data.reason, data.retryAfterMs);
      }
      this.triggerReconnect();
    });

    this.signaling.on('joined', (data) => {
      if (this.room !== null) {
        this.room.handleJoined(data.participantId, data.roomId);
      }
    });

    this.signaling.on('left', (data) => {
      if (this.room !== null) {
        this.room.handleLeft(data.reason);
      }
    });

    this.signaling.on('offer', async (data) => {
      await this.handleServerOffer(data.sdp);
    });

    this.signaling.on('candidate', async (data) => {
      await this.handleServerCandidate(data);
    });

    this.signaling.on('recordingStarted', (data) => {
      if (this.room !== null) {
        this.room.handleRecordingStarted(data.recordingId, data.startedBy);
      }
    });

    this.signaling.on('recordingStopped', (data) => {
      if (this.room !== null) {
        this.room.handleRecordingStopped(data.recordingId, data.stoppedBy);
      }
    });
  }

  /**
   * Connect to signaling server
   */
  public async connect(url?: string): Promise<void> {
    const connectUrl = url ?? this.config.url;
    this.setConnectionState('connecting');
    this.emit('connecting', undefined);

    try {
      await this.signaling.connect(connectUrl);
    } catch (error) {
      this.setConnectionState('disconnected');
      throw error;
    }
  }

  /**
   * Join a room
   */
  public async join(token: string, options: JoinOptions = {}): Promise<Room> {
    if (!this.signaling.isConnected()) {
      throw new SFUError(ErrorCodes.CONNECTION_FAILED, 'Not connected to signaling server');
    }

    // Store token for reconnection
    this.lastToken = token;

    this.logger.info('Joining room');

    // Join via signaling
    const result = await this.signaling.join({
      token,
      sessionId: options.sessionId,
      metadata: options.metadata,
    });

    // Update ICE servers
    if (result.iceServers.length > 0) {
      this.config.iceServers = result.iceServers;
    }

    // Create peer connection
    this.peerConnection = new PeerConnection({
      iceServers: this.config.iceServers,
    });

    this.setupPeerConnectionHandlers();

    // Create local participant
    const localParticipant = new LocalParticipant(
      result.participantId,
      options.metadata
    );
    this.setupLocalParticipant(localParticipant);

    // Create room
    this.room = new Room(
      result.roomId,
      result.sessionId,
      localParticipant
    );

    this.room.setLeaveCallback(async () => {
      await this.leaveRoom();
    });

    // Add existing participants
    for (const participantData of result.participants) {
      const participant = this.createRemoteParticipant(participantData);
      this.room.addParticipant(participant);
    }

    this.room.updateState('joined');

    // Auto-subscribe if requested
    if (options.autoSubscribe === true) {
      await this.autoSubscribeAll();
    }

    this.logger.info('Joined room', {
      roomId: result.roomId,
      participantId: result.participantId,
      reconnected: result.reconnected,
    });

    return this.room;
  }

  /**
   * Disconnect from server
   */
  public disconnect(): void {
    this.logger.info('Disconnecting');

    if (this.room !== null) {
      this.room.cleanup();
      this.room = null;
    }

    if (this.peerConnection !== null) {
      this.peerConnection.close();
      this.peerConnection = null;
    }

    // Clear pending subscriptions on disconnect
    this.pendingSubscriptions.clear();

    this.signaling.disconnect();
    this.setConnectionState('disconnected');
  }

  /**
   * Get current connection state
   */
  public getConnectionState(): ConnectionState {
    return this.connectionState;
  }

  /**
   * Get current room
   */
  public getRoom(): Room | null {
    return this.room;
  }

  // ---- Private methods ----

  /**
   * Set up peer connection handlers
   */
  private setupPeerConnectionHandlers(): void {
    if (this.peerConnection === null) {
      return;
    }

    this.peerConnection.on('track', (event) => {
      this.handleIncomingTrack(event);
    });

    this.peerConnection.on('iceCandidate', async (candidate) => {
      try {
        await this.signaling.candidate({
          candidate: candidate.candidate,
          sdpMid: candidate.sdpMid ?? undefined,
          sdpMLineIndex: candidate.sdpMLineIndex ?? undefined,
        });
      } catch (error) {
        this.logger.error('Failed to send ICE candidate', error);
      }
    });

    this.peerConnection.on('connectionStateChange', (state) => {
      this.logger.debug('Peer connection state', { state });
      if (state === 'failed' || state === 'disconnected') {
        this.triggerReconnect();
      }
    });

    this.peerConnection.on('negotiationNeeded', async () => {
      await this.renegotiate();
    });

    this.peerConnection.on('iceRestart', async () => {
      await this.restartIce();
    });
  }

  /**
   * Set up local participant callbacks
   */
  private setupLocalParticipant(participant: LocalParticipant): void {
    participant.setPublishCallback(async (track, options) => {
      return this.publishTrack(track, options);
    });

    participant.setUnpublishCallback(async (trackId) => {
      await this.unpublishTrack(trackId);
    });
  }

  /**
   * Create a remote participant from data
   */
  private createRemoteParticipant(data: ParticipantData): RemoteParticipant {
    const participant = new RemoteParticipant(data.id, data.metadata);

    participant.setCallbacks({
      subscribe: async (trackId, options) => {
        return this.subscribeToTrack(trackId, options);
      },
      unsubscribe: async (subscriptionId) => {
        await this.unsubscribeFromTrack(subscriptionId);
      },
      setLayer: async (trackId, layer) => {
        await this.setPreferredLayer(trackId, layer);
      },
    });

    // Add existing tracks
    for (const trackData of data.tracks) {
      participant.addTrack(
        trackData.id,
        trackData.kind,
        trackData.simulcast,
        trackData.metadata,
        trackData.name
      );
    }

    return participant;
  }

  /**
   * Publish a track
   */
  private async publishTrack(
    track: MediaStreamTrack,
    options: PublishOptions
  ): Promise<string> {
    if (this.peerConnection === null) {
      throw new SFUError(ErrorCodes.CONNECTION_FAILED, 'No peer connection');
    }

    // Notify server about publish first to get server track ID
    const result = await this.signaling.publish({
      kind: track.kind as 'audio' | 'video',
      simulcast: options.simulcast,
      metadata: options.metadata,
      label: options.name,
    });

    // Add track to peer connection with server track ID mapping
    const stream = new MediaStream([track]);
    this.peerConnection.addTrack(track, stream, {
      simulcast: options.simulcast,
      serverTrackId: result.trackId,
    });

    // Trigger renegotiation
    await this.renegotiate();

    return result.trackId;
  }

  /**
   * Unpublish a track by server track ID
   */
  private async unpublishTrack(serverTrackId: string): Promise<void> {
    if (this.peerConnection === null) {
      return;
    }

    await this.signaling.unpublish({ trackId: serverTrackId });
    // Use removeTrackByServerId to correctly find and remove the track
    this.peerConnection.removeTrackByServerId(serverTrackId);
    await this.renegotiate();
  }

  /**
   * Subscribe to a track
   */
  private async subscribeToTrack(
    trackId: string,
    options: SubscribeOptions
  ): Promise<{ subscriptionId: string }> {
    // Find the track's publisher
    let publisherId: string | undefined;
    if (this.room !== null) {
      for (const p of this.room.participants) {
        const track = p.getTrack(trackId);
        if (track !== undefined) {
          publisherId = p.id;
          break;
        }
      }
    }

    if (publisherId === undefined) {
      throw new SFUError(
        ErrorCodes.TRACK_NOT_FOUND,
        `Track not found: ${trackId}`
      );
    }

    const result = await this.signaling.subscribe({
      publisherId,
      trackId,
      preferredLayer: options.preferredLayer,
    });

    // Store pending subscription for track matching
    this.pendingSubscriptions.set(trackId, result.subscriptionId);

    return { subscriptionId: result.subscriptionId };
  }

  /**
   * Unsubscribe from a track
   */
  private async unsubscribeFromTrack(subscriptionId: string): Promise<void> {
    // Clean up pending subscription if exists
    for (const [trackId, subId] of this.pendingSubscriptions) {
      if (subId === subscriptionId) {
        this.pendingSubscriptions.delete(trackId);
        break;
      }
    }
    await this.signaling.unsubscribe({ subscriptionId });
  }

  /**
   * Set preferred simulcast layer
   */
  private async setPreferredLayer(
    trackId: string,
    layer: SimulcastLayer
  ): Promise<void> {
    await this.signaling.setPreferredLayer({ trackId, layer });
  }

  /**
   * Handle incoming track from peer connection
   */
  private handleIncomingTrack(event: RTCTrackEvent): void {
    const incomingTrack = event.track;
    const transceiver = event.transceiver;

    this.logger.debug('Incoming track', {
      kind: incomingTrack.kind,
      mid: transceiver?.mid,
      streams: event.streams.length,
    });

    if (this.room === null) {
      return;
    }

    // Strategy 1: Match using subscriptionId (most reliable)
    // Server sends trackSubscribed notification with subscriptionId before ontrack fires
    // We use this to do 1:1 matching
    for (const participant of this.room.participants) {
      for (const remoteTrack of participant.tracks) {
        if (
          remoteTrack.kind === incomingTrack.kind &&
          !remoteTrack.isSubscribed &&
          remoteTrack.subscriptionId !== undefined
        ) {
          this.logger.debug('Matched track via subscriptionId', {
            trackId: remoteTrack.id,
            subscriptionId: remoteTrack.subscriptionId,
            participantId: participant.id,
          });
          // Clean up pending subscription if exists
          this.pendingSubscriptions.delete(remoteTrack.id);
          this.room.handleTrackSubscribed(
            participant.id,
            remoteTrack.id,
            incomingTrack
          );
          return;
        }
      }
    }

    // Strategy 2: Fallback using pending subscriptions map
    // This handles cases where ontrack fires before trackSubscribed notification
    for (const [trackId, subscriptionId] of this.pendingSubscriptions) {
      for (const participant of this.room.participants) {
        const remoteTrack = participant.getTrack(trackId);
        if (
          remoteTrack !== undefined &&
          remoteTrack.kind === incomingTrack.kind &&
          !remoteTrack.isSubscribed
        ) {
          this.logger.debug('Matched track via pending subscription', {
            trackId,
            subscriptionId,
            participantId: participant.id,
          });
          this.pendingSubscriptions.delete(trackId);
          // Set subscriptionId on the track for future reference
          remoteTrack.subscriptionId = subscriptionId;
          this.room.handleTrackSubscribed(
            participant.id,
            remoteTrack.id,
            incomingTrack
          );
          return;
        }
      }
    }

    this.logger.warn('Could not match incoming track', {
      kind: incomingTrack.kind,
      mid: transceiver?.mid,
      pendingCount: this.pendingSubscriptions.size,
    });
  }

  /**
   * Handle server-initiated offer
   */
  private async handleServerOffer(sdp: string): Promise<void> {
    if (this.peerConnection === null) {
      return;
    }

    await this.peerConnection.setRemoteDescription({ type: 'offer', sdp });
    const answer = await this.peerConnection.createAnswer();
    await this.peerConnection.setLocalDescription(answer);

    if (answer.sdp !== undefined) {
      await this.signaling.answer({ sdp: answer.sdp });
    }
  }

  /**
   * Handle server ICE candidate
   */
  private async handleServerCandidate(data: {
    candidate: string;
    sdpMid?: string;
    sdpMLineIndex?: number;
  }): Promise<void> {
    if (this.peerConnection === null) {
      return;
    }

    await this.peerConnection.addIceCandidate({
      candidate: data.candidate,
      sdpMid: data.sdpMid,
      sdpMLineIndex: data.sdpMLineIndex,
    });
  }

  /**
   * Renegotiate connection
   */
  private async renegotiate(): Promise<void> {
    if (this.peerConnection === null) {
      return;
    }

    const offer = await this.peerConnection.createOffer();
    await this.peerConnection.setLocalDescription(offer);

    if (offer.sdp !== undefined) {
      const result = await this.signaling.offer({ sdp: offer.sdp });
      await this.peerConnection.setRemoteDescription({
        type: 'answer',
        sdp: result.sdp,
      });
    }
  }

  /**
   * Restart ICE
   */
  private async restartIce(): Promise<void> {
    if (this.peerConnection === null) {
      return;
    }

    const offer = await this.peerConnection.restartIce();
    await this.peerConnection.setLocalDescription(offer);

    if (offer.sdp !== undefined) {
      const result = await this.signaling.offer({ sdp: offer.sdp });
      await this.peerConnection.setRemoteDescription({
        type: 'answer',
        sdp: result.sdp,
      });
    }
  }

  /**
   * Auto-subscribe to all available tracks
   */
  private async autoSubscribeAll(): Promise<void> {
    if (this.room === null) {
      return;
    }

    for (const participant of this.room.participants) {
      for (const track of participant.tracks) {
        try {
          await participant.subscribe(track.id);
        } catch (error) {
          this.logger.warn('Failed to auto-subscribe to track', {
            trackId: track.id,
            error,
          });
        }
      }
    }
  }

  /**
   * Leave the current room
   */
  private async leaveRoom(): Promise<void> {
    if (this.room === null) {
      return;
    }

    await this.signaling.leave();

    if (this.peerConnection !== null) {
      this.peerConnection.close();
      this.peerConnection = null;
    }

    // Clear pending subscriptions on leave
    this.pendingSubscriptions.clear();

    this.room.cleanup();
    this.room = null;
  }

  /**
   * Handle disconnection
   */
  private handleDisconnect(event: CloseEvent): void {
    this.logger.info('Disconnected', { code: event.code, reason: event.reason });

    const reason = this.mapCloseReason(event);
    this.setConnectionState('disconnected');

    if (this.room !== null) {
      this.room.handleDisconnected(reason);
    }

    this.emit('disconnected', reason);

    // Attempt reconnect if enabled
    if (this.config.autoReconnect && this.shouldReconnect(event)) {
      this.triggerReconnect();
    }
  }

  /**
   * Trigger reconnection
   */
  private triggerReconnect(): void {
    if (this.isReconnecting) {
      return;
    }

    if (this.reconnectAttempt >= this.config.reconnect.maxAttempts) {
      this.logger.warn('Max reconnect attempts reached');
      return;
    }

    this.isReconnecting = true;
    this.setConnectionState('reconnecting');
    this.emit('reconnecting', undefined);

    if (this.room !== null) {
      this.room.handleReconnecting();
    }

    void this.attemptReconnect();
  }

  /**
   * Attempt to reconnect
   */
  private async attemptReconnect(): Promise<void> {
    const sessionId = this.room?.sessionId;
    const token = this.lastToken;

    if (token === null) {
      this.logger.warn('Cannot reconnect: no token stored');
      this.isReconnecting = false;
      this.emit(
        'error',
        new SFUError(ErrorCodes.CONNECTION_FAILED, 'No token available for reconnection')
      );
      return;
    }

    try {
      await retry(
        async () => {
          this.reconnectAttempt++;
          this.logger.info('Reconnect attempt', {
            attempt: this.reconnectAttempt,
          });

          // Clean up existing peer connection before reconnect
          if (this.peerConnection !== null) {
            this.peerConnection.close();
            this.peerConnection = null;
          }

          // Clean up existing room to prevent listener/track leaks
          if (this.room !== null) {
            this.room.cleanup();
            this.room = null;
          }

          // Clear pending subscriptions
          this.pendingSubscriptions.clear();

          await this.connect();

          if (sessionId !== undefined) {
            await this.join(token, { sessionId });
          }
        },
        this.config.reconnect,
        (attempt, error, nextDelay) => {
          this.logger.debug('Reconnect retry', {
            attempt,
            error: error.message,
            nextDelay,
          });
        }
      );

      this.reconnectAttempt = 0;
      this.isReconnecting = false;
      this.emit('reconnected', undefined);

      if (this.room !== null) {
        this.room.handleReconnected();
      }
    } catch (error) {
      this.logger.error('Reconnect failed', error);
      this.isReconnecting = false;
      this.emit(
        'error',
        new SFUError(ErrorCodes.CONNECTION_FAILED, 'Reconnect failed')
      );
    }
  }

  /**
   * Set connection state
   */
  private setConnectionState(state: ConnectionState): void {
    this.connectionState = state;
  }

  /**
   * Map close event to disconnect reason
   */
  private mapCloseReason(event: CloseEvent): DisconnectReason {
    if (event.code === 1000) {
      return 'client_initiated';
    }
    if (event.code === 1001) {
      return 'server_shutdown';
    }
    return 'connection_error';
  }

  /**
   * Check if should attempt reconnect
   */
  private shouldReconnect(event: CloseEvent): boolean {
    // Don't reconnect on clean close
    if (event.code === 1000) {
      return false;
    }
    return true;
  }
}
