/**
 * Room class - manages room state, participants, and media
 */

import { EventEmitter } from '../events/EventEmitter';
import { LocalParticipant, RemoteParticipant } from './Participant';
import { RemoteTrack } from '../media/RemoteTrack';
import type {
  RoomState,
  ServerRoomState,
  DisconnectReason,
  ParticipantLeaveReason,
  LayerChangedEvent,
  ConnectionQuality,
  ServerError,
  ReconnectReason,
  LeaveReason,
} from '../utils/types';

/** Room events */
export interface RoomEvents {
  stateChanged: RoomState;
  serverStateChanged: ServerRoomState;
  participantJoined: RemoteParticipant;
  participantLeft: { participant: RemoteParticipant; reason: ParticipantLeaveReason };
  trackPublished: { track: RemoteTrack; participant: RemoteParticipant };
  trackUnpublished: { track: RemoteTrack; participant: RemoteParticipant };
  trackSubscribed: { track: RemoteTrack; participant: RemoteParticipant };
  trackSubscriptionFailed: { trackId: string; error: Error };
  layerChanged: LayerChangedEvent;
  connectionQualityChanged: { quality: ConnectionQuality; participant: LocalParticipant | RemoteParticipant };
  reconnecting: void;
  reconnected: void;
  disconnected: DisconnectReason;
  joined: { participantId: string; roomId: string };
  left: LeaveReason;
  error: ServerError;
  reconnectRequested: { reason: ReconnectReason; retryAfterMs: number };
  recordingStarted: { recordingId: string; startedBy: string };
  recordingStopped: { recordingId: string; stoppedBy: string };
}

/**
 * Room class
 */
export class Room extends EventEmitter<RoomEvents> {
  /** Room ID */
  public readonly id: string;

  /** Session ID for reconnection */
  public readonly sessionId: string;

  /** Local participant */
  public readonly localParticipant: LocalParticipant;

  /** Room state (client-side) */
  private _state: RoomState = 'disconnected';

  /** Server-side room state */
  private _serverState: ServerRoomState = 'created';

  /** Room metadata */
  private _metadata: Record<string, unknown> = {};

  /** Remote participants */
  private _participants: Map<string, RemoteParticipant> = new Map();

  /** Leave callback */
  private leaveCallback?: () => Promise<void>;

  constructor(
    id: string,
    sessionId: string,
    localParticipant: LocalParticipant,
    metadata: Record<string, unknown> = {}
  ) {
    super();
    this.id = id;
    this.sessionId = sessionId;
    this.localParticipant = localParticipant;
    this._metadata = metadata;
  }

  /**
   * Get room state
   */
  public get state(): RoomState {
    return this._state;
  }

  /**
   * Get server state
   */
  public get serverState(): ServerRoomState {
    return this._serverState;
  }

  /**
   * Get room metadata
   */
  public get metadata(): Record<string, unknown> {
    return { ...this._metadata };
  }

  /**
   * Get remote participants
   */
  public get participants(): RemoteParticipant[] {
    return Array.from(this._participants.values());
  }

  /**
   * Set leave callback
   */
  public setLeaveCallback(callback: () => Promise<void>): void {
    this.leaveCallback = callback;
  }

  /**
   * Update room state
   */
  public updateState(state: RoomState): void {
    if (this._state !== state) {
      this._state = state;
      this.emit('stateChanged', state);
    }
  }

  /**
   * Update server state
   */
  public updateServerState(state: ServerRoomState): void {
    if (this._serverState !== state) {
      this._serverState = state;
      this.emit('serverStateChanged', state);
    }
  }

  /**
   * Leave the room
   */
  public async leave(): Promise<void> {
    if (this.leaveCallback !== undefined) {
      await this.leaveCallback();
    }
  }

  /**
   * Get a participant by ID
   */
  public getParticipant(id: string): RemoteParticipant | undefined {
    return this._participants.get(id);
  }

  /**
   * Add a remote participant
   */
  public addParticipant(participant: RemoteParticipant): void {
    this._participants.set(participant.id, participant);
    this.emit('participantJoined', participant);
  }

  /**
   * Remove a remote participant
   */
  public removeParticipant(
    participantId: string,
    reason: ParticipantLeaveReason
  ): RemoteParticipant | undefined {
    const participant = this._participants.get(participantId);
    if (participant !== undefined) {
      participant.clearTracks();
      this._participants.delete(participantId);
      this.emit('participantLeft', { participant, reason });
    }
    return participant;
  }

  /**
   * Handle track published notification
   */
  public handleTrackPublished(
    publisherId: string,
    trackId: string,
    kind: 'audio' | 'video',
    simulcast: boolean,
    metadata: Record<string, unknown>,
    name?: string
  ): void {
    const participant = this._participants.get(publisherId);
    if (participant !== undefined) {
      const track = participant.addTrack(trackId, kind, simulcast, metadata, name);
      this.emit('trackPublished', { track, participant });
    }
  }

  /**
   * Handle track unpublished notification
   */
  public handleTrackUnpublished(publisherId: string, trackId: string): void {
    const participant = this._participants.get(publisherId);
    if (participant !== undefined) {
      const track = participant.removeTrack(trackId);
      if (track !== undefined) {
        this.emit('trackUnpublished', { track, participant });
      }
    }
  }

  /**
   * Handle track subscribed
   */
  public handleTrackSubscribed(
    publisherId: string,
    trackId: string,
    mediaStreamTrack: MediaStreamTrack
  ): void {
    const participant = this._participants.get(publisherId);
    if (participant !== undefined) {
      const track = participant.getTrack(trackId);
      if (track !== undefined) {
        track.setMediaStreamTrack(mediaStreamTrack);
        this.emit('trackSubscribed', { track, participant });
      }
    }
  }

  /**
   * Handle track subscription confirmed (sets subscriptionId on track)
   */
  public handleTrackSubscriptionConfirmed(
    publisherId: string,
    trackId: string,
    subscriptionId: string
  ): void {
    const participant = this._participants.get(publisherId);
    if (participant !== undefined) {
      const track = participant.getTrack(trackId);
      if (track !== undefined) {
        track.subscriptionId = subscriptionId;
      }
    }
  }

  /**
   * Handle track subscription failed
   */
  public handleTrackSubscriptionFailed(trackId: string, error: Error): void {
    this.emit('trackSubscriptionFailed', { trackId, error });
  }

  /**
   * Handle layer changed notification
   */
  public handleLayerChanged(event: LayerChangedEvent): void {
    // Find and update the track
    for (const participant of this._participants.values()) {
      const track = participant.getTrack(event.trackId);
      if (track !== undefined) {
        track.updateCurrentLayer(event.actualLayer);
        break;
      }
    }
    this.emit('layerChanged', event);
  }

  /**
   * Handle connection quality changed
   */
  public handleConnectionQualityChanged(
    participantId: string,
    quality: ConnectionQuality
  ): void {
    if (participantId === this.localParticipant.id) {
      this.emit('connectionQualityChanged', {
        quality,
        participant: this.localParticipant,
      });
    } else {
      const participant = this._participants.get(participantId);
      if (participant !== undefined) {
        participant.updateConnectionQuality(quality);
        this.emit('connectionQualityChanged', { quality, participant });
      }
    }
  }

  /**
   * Handle server error
   */
  public handleServerError(error: ServerError): void {
    this.emit('error', error);
  }

  /**
   * Handle reconnect request
   */
  public handleReconnectRequested(
    reason: ReconnectReason,
    retryAfterMs: number
  ): void {
    this.emit('reconnectRequested', { reason, retryAfterMs });
  }

  /**
   * Handle joined notification
   */
  public handleJoined(participantId: string, roomId: string): void {
    this.emit('joined', { participantId, roomId });
  }

  /**
   * Handle left notification
   */
  public handleLeft(reason: LeaveReason): void {
    this.emit('left', reason);
  }

  /**
   * Handle recording started
   */
  public handleRecordingStarted(recordingId: string, startedBy: string): void {
    this.emit('recordingStarted', { recordingId, startedBy });
  }

  /**
   * Handle recording stopped
   */
  public handleRecordingStopped(recordingId: string, stoppedBy: string): void {
    this.emit('recordingStopped', { recordingId, stoppedBy });
  }

  /**
   * Handle disconnection
   */
  public handleDisconnected(reason: DisconnectReason): void {
    this.updateState('disconnected');
    this.emit('disconnected', reason);
  }

  /**
   * Handle reconnecting state
   */
  public handleReconnecting(): void {
    this.updateState('reconnecting');
    this.emit('reconnecting', undefined);
  }

  /**
   * Handle reconnected state
   */
  public handleReconnected(): void {
    this.updateState('joined');
    this.emit('reconnected', undefined);
  }

  /**
   * Clean up room resources
   */
  public cleanup(): void {
    this.localParticipant.clearTracks();
    for (const participant of this._participants.values()) {
      participant.clearTracks();
    }
    this._participants.clear();
  }
}
