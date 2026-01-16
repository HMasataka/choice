/**
 * WebRTC PeerConnection wrapper
 */

import { EventEmitter } from '../events/EventEmitter';
import { ICEManager, type ICEManagerOptions } from './ICEManager';
import { Logger } from '../utils/logger';
import { createSimulcastEncodings, type SimulcastLayerConfig } from '../media/Simulcast';
import type { LoggerConfig } from '../utils/types';

/** Peer connection events */
interface PeerConnectionEvents {
  track: RTCTrackEvent;
  iceCandidate: RTCIceCandidate;
  connectionStateChange: RTCPeerConnectionState;
  iceConnectionStateChange: RTCIceConnectionState;
  signalingStateChange: RTCSignalingState;
  negotiationNeeded: void;
  iceRestart: void;
}

/** Peer connection options */
export interface PeerConnectionOptions extends ICEManagerOptions {
  logger?: LoggerConfig;
}

/**
 * WebRTC PeerConnection wrapper with ICE management
 */
export class PeerConnection extends EventEmitter<PeerConnectionEvents> {
  private pc: RTCPeerConnection;
  private readonly iceManager: ICEManager;
  private readonly logger: Logger;
  /** Map from browser track ID to sender */
  private sendersByBrowserId: Map<string, RTCRtpSender> = new Map();
  /** Map from server track ID to browser track ID */
  private serverToBrowserId: Map<string, string> = new Map();
  private closed = false;

  constructor(options: PeerConnectionOptions = {}) {
    super();
    this.iceManager = new ICEManager(options);
    this.logger = new Logger(options.logger, 'PC');

    // Create peer connection
    this.pc = new RTCPeerConnection(this.iceManager.getConfiguration());
    this.setupEventHandlers();
  }

  /**
   * Set up peer connection event handlers
   */
  private setupEventHandlers(): void {
    this.pc.ontrack = (event) => {
      this.logger.debug('Track received', { kind: event.track.kind });
      this.emit('track', event);
    };

    this.pc.onicecandidate = (event) => {
      this.iceManager.handleIceCandidate(event.candidate);
      if (event.candidate !== null) {
        this.emit('iceCandidate', event.candidate);
      }
    };

    this.pc.onconnectionstatechange = () => {
      this.logger.info('Connection state changed', { state: this.pc.connectionState });
      this.emit('connectionStateChange', this.pc.connectionState);
    };

    this.pc.oniceconnectionstatechange = () => {
      this.iceManager.handleConnectionStateChange(this.pc.iceConnectionState);
      this.emit('iceConnectionStateChange', this.pc.iceConnectionState);

      // Auto-trigger ICE restart on failure
      if (this.iceManager.needsRestart()) {
        this.emit('iceRestart', undefined);
      }
    };

    this.pc.onicegatheringstatechange = () => {
      this.iceManager.handleGatheringStateChange(this.pc.iceGatheringState);
    };

    this.pc.onsignalingstatechange = () => {
      this.logger.debug('Signaling state changed', { state: this.pc.signalingState });
      this.emit('signalingStateChange', this.pc.signalingState);
    };

    this.pc.onnegotiationneeded = () => {
      this.logger.debug('Negotiation needed');
      this.emit('negotiationNeeded', undefined);
    };
  }

  /**
   * Update ICE servers
   */
  public setIceServers(servers: RTCIceServer[]): void {
    this.iceManager.setIceServers(servers);
    // Note: RTCPeerConnection.setConfiguration is not well supported
    // ICE restart is typically needed to apply new servers
  }

  /**
   * Add a track to the peer connection
   * @param track - The MediaStreamTrack to add
   * @param stream - The MediaStream containing the track
   * @param options - Options including simulcast and server track ID
   * @returns The RTCRtpSender for the track
   */
  public addTrack(
    track: MediaStreamTrack,
    stream: MediaStream,
    options?: {
      simulcast?: boolean;
      simulcastLayers?: SimulcastLayerConfig[];
      serverTrackId?: string;
    }
  ): RTCRtpSender {
    this.logger.debug('Adding track', {
      kind: track.kind,
      browserTrackId: track.id,
      serverTrackId: options?.serverTrackId,
      simulcast: options?.simulcast,
    });

    let sender: RTCRtpSender;

    if (options?.simulcast === true && track.kind === 'video') {
      // Add with simulcast
      const encodings = createSimulcastEncodings(options.simulcastLayers);
      const transceiver = this.pc.addTransceiver(track, {
        direction: 'sendonly',
        streams: [stream],
        sendEncodings: encodings,
      });
      sender = transceiver.sender;
    } else {
      // Add normally
      sender = this.pc.addTrack(track, stream);
    }

    // Store sender by browser track ID
    this.sendersByBrowserId.set(track.id, sender);

    // Store mapping from server track ID to browser track ID
    if (options?.serverTrackId !== undefined) {
      this.serverToBrowserId.set(options.serverTrackId, track.id);
    }

    return sender;
  }

  /**
   * Remove a track from the peer connection by server track ID
   * @param serverTrackId - The server-assigned track ID
   */
  public removeTrackByServerId(serverTrackId: string): void {
    const browserTrackId = this.serverToBrowserId.get(serverTrackId);
    if (browserTrackId === undefined) {
      this.logger.warn('Server track ID not found in mapping', { serverTrackId });
      return;
    }

    const sender = this.sendersByBrowserId.get(browserTrackId);
    if (sender !== undefined) {
      this.logger.debug('Removing track', { serverTrackId, browserTrackId });
      this.pc.removeTrack(sender);
    } else {
      this.logger.warn('Sender not found for browser track ID', { browserTrackId, serverTrackId });
    }
    // Always clean up maps even if sender is not found to prevent stale entries
    this.sendersByBrowserId.delete(browserTrackId);
    this.serverToBrowserId.delete(serverTrackId);
  }

  /**
   * Remove a track from the peer connection by browser track ID
   * @param browserTrackId - The browser MediaStreamTrack.id
   */
  public removeTrack(browserTrackId: string): void {
    const sender = this.sendersByBrowserId.get(browserTrackId);
    if (sender !== undefined) {
      this.logger.debug('Removing track by browser ID', { browserTrackId });
      this.pc.removeTrack(sender);
      this.sendersByBrowserId.delete(browserTrackId);

      // Also remove from server mapping
      for (const [serverId, browserId] of this.serverToBrowserId) {
        if (browserId === browserTrackId) {
          this.serverToBrowserId.delete(serverId);
          break;
        }
      }
    }
  }

  /**
   * Register server track ID for an existing track
   * @param browserTrackId - The browser MediaStreamTrack.id
   * @param serverTrackId - The server-assigned track ID
   */
  public registerServerTrackId(browserTrackId: string, serverTrackId: string): void {
    this.serverToBrowserId.set(serverTrackId, browserTrackId);
    this.logger.debug('Registered server track ID', { browserTrackId, serverTrackId });
  }

  /**
   * Get sender for a track by browser track ID
   */
  public getSender(browserTrackId: string): RTCRtpSender | undefined {
    return this.sendersByBrowserId.get(browserTrackId);
  }

  /**
   * Get sender for a track by server track ID
   */
  public getSenderByServerId(serverTrackId: string): RTCRtpSender | undefined {
    const browserTrackId = this.serverToBrowserId.get(serverTrackId);
    if (browserTrackId === undefined) {
      return undefined;
    }
    return this.sendersByBrowserId.get(browserTrackId);
  }

  /**
   * Create an SDP offer
   */
  public async createOffer(options?: RTCOfferOptions): Promise<RTCSessionDescriptionInit> {
    this.logger.debug('Creating offer');
    const offer = await this.pc.createOffer(options);
    return offer;
  }

  /**
   * Create an SDP answer
   */
  public async createAnswer(options?: RTCAnswerOptions): Promise<RTCSessionDescriptionInit> {
    this.logger.debug('Creating answer');
    const answer = await this.pc.createAnswer(options);
    return answer;
  }

  /**
   * Set local description
   */
  public async setLocalDescription(description: RTCSessionDescriptionInit): Promise<void> {
    this.logger.debug('Setting local description', { type: description.type });
    await this.pc.setLocalDescription(description);
  }

  /**
   * Set remote description
   */
  public async setRemoteDescription(description: RTCSessionDescriptionInit): Promise<void> {
    this.logger.debug('Setting remote description', { type: description.type });
    await this.pc.setRemoteDescription(description);
  }

  /**
   * Add ICE candidate
   */
  public async addIceCandidate(candidate: RTCIceCandidateInit): Promise<void> {
    this.logger.debug('Adding ICE candidate');
    await this.pc.addIceCandidate(candidate);
  }

  /**
   * Restart ICE
   */
  public async restartIce(): Promise<RTCSessionDescriptionInit> {
    this.logger.info('Restarting ICE');
    this.iceManager.triggerRestart();
    return this.createOffer({ iceRestart: true });
  }

  /**
   * Get connection state
   */
  public getConnectionState(): RTCPeerConnectionState {
    return this.pc.connectionState;
  }

  /**
   * Get ICE connection state
   */
  public getIceConnectionState(): RTCIceConnectionState {
    return this.pc.iceConnectionState;
  }

  /**
   * Get signaling state
   */
  public getSignalingState(): RTCSignalingState {
    return this.pc.signalingState;
  }

  /**
   * Get local description
   */
  public getLocalDescription(): RTCSessionDescription | null {
    return this.pc.localDescription;
  }

  /**
   * Get remote description
   */
  public getRemoteDescription(): RTCSessionDescription | null {
    return this.pc.remoteDescription;
  }

  /**
   * Get stats
   */
  public async getStats(selector?: MediaStreamTrack | null): Promise<RTCStatsReport> {
    return this.pc.getStats(selector);
  }

  /**
   * Get all transceivers
   */
  public getTransceivers(): RTCRtpTransceiver[] {
    return this.pc.getTransceivers();
  }

  /**
   * Check if closed
   */
  public isClosed(): boolean {
    return this.closed;
  }

  /**
   * Close the peer connection
   */
  public close(): void {
    if (!this.closed) {
      this.logger.info('Closing peer connection');
      this.closed = true;
      this.pc.close();
      this.sendersByBrowserId.clear();
      this.serverToBrowserId.clear();
      this.iceManager.reset();
    }
  }
}
