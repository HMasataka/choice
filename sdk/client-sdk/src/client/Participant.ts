/**
 * Participant classes (Local and Remote)
 */

import { EventEmitter } from '../events/EventEmitter';
import { LocalTrack } from '../media/LocalTrack';
import { RemoteTrack } from '../media/RemoteTrack';
import type {
  ConnectionQuality,
  TrackKind,
  SimulcastLayer,
  PublishOptions,
  SubscribeOptions,
} from '../utils/types';

/** Base participant events */
interface ParticipantEvents {
  trackAdded: LocalTrack | RemoteTrack;
  trackRemoved: LocalTrack | RemoteTrack;
  metadataChanged: Record<string, unknown>;
}

/** Local participant events */
interface LocalParticipantEvents extends ParticipantEvents {
  trackPublished: LocalTrack;
  trackUnpublished: LocalTrack;
}

/** Remote participant events */
interface RemoteParticipantEvents extends ParticipantEvents {
  trackSubscribed: RemoteTrack;
  trackUnsubscribed: RemoteTrack;
  connectionQualityChanged: ConnectionQuality;
}

/**
 * Base participant class
 */
abstract class BaseParticipant<
  T extends ParticipantEvents
> extends EventEmitter<T> {
  public readonly id: string;
  protected _metadata: Record<string, unknown>;

  constructor(id: string, metadata: Record<string, unknown> = {}) {
    super();
    this.id = id;
    this._metadata = metadata;
  }

  public get metadata(): Record<string, unknown> {
    return { ...this._metadata };
  }

  public updateMetadata(metadata: Record<string, unknown>): void {
    this._metadata = { ...this._metadata, ...metadata };
    this.emit('metadataChanged' as keyof T, this._metadata as T[keyof T]);
  }
}

/** Publish callback type */
export type PublishCallback = (
  track: MediaStreamTrack,
  options: PublishOptions
) => Promise<string>;

/** Unpublish callback type */
export type UnpublishCallback = (trackId: string) => Promise<void>;

/**
 * Local participant (self)
 */
export class LocalParticipant extends BaseParticipant<LocalParticipantEvents> {
  private _tracks: Map<string, LocalTrack> = new Map();
  private publishCallback?: PublishCallback;
  private unpublishCallback?: UnpublishCallback;

  constructor(id: string, metadata: Record<string, unknown> = {}) {
    super(id, metadata);
  }

  /**
   * Get published tracks
   */
  public get tracks(): LocalTrack[] {
    return Array.from(this._tracks.values());
  }

  /**
   * Set publish callback
   */
  public setPublishCallback(callback: PublishCallback): void {
    this.publishCallback = callback;
  }

  /**
   * Set unpublish callback
   */
  public setUnpublishCallback(callback: UnpublishCallback): void {
    this.unpublishCallback = callback;
  }

  /**
   * Publish a track
   */
  public async publish(
    track: MediaStreamTrack,
    options: PublishOptions = {}
  ): Promise<LocalTrack> {
    if (this.publishCallback === undefined) {
      throw new Error('Publish callback not set');
    }

    const localTrack = new LocalTrack(track, options);

    // Publish to server and get track ID
    const trackId = await this.publishCallback(track, options);
    localTrack.id = trackId;

    this._tracks.set(trackId, localTrack);
    this.emit('trackAdded', localTrack);
    this.emit('trackPublished', localTrack);

    return localTrack;
  }

  /**
   * Unpublish a track
   */
  public async unpublish(track: LocalTrack): Promise<void> {
    if (track.id === null) {
      throw new Error('Track has not been published');
    }

    if (this.unpublishCallback === undefined) {
      throw new Error('Unpublish callback not set');
    }

    await this.unpublishCallback(track.id);

    this._tracks.delete(track.id);
    track.stop();
    this.emit('trackRemoved', track);
    this.emit('trackUnpublished', track);
  }

  /**
   * Set microphone enabled state
   */
  public async setMicrophoneEnabled(enabled: boolean): Promise<void> {
    for (const track of this._tracks.values()) {
      if (track.kind === 'audio') {
        if (enabled) {
          track.unmute();
        } else {
          track.mute();
        }
      }
    }
  }

  /**
   * Set camera enabled state
   */
  public async setCameraEnabled(enabled: boolean): Promise<void> {
    for (const track of this._tracks.values()) {
      if (track.kind === 'video') {
        if (enabled) {
          track.unmute();
        } else {
          track.mute();
        }
      }
    }
  }

  /**
   * Get track by ID
   */
  public getTrack(trackId: string): LocalTrack | undefined {
    return this._tracks.get(trackId);
  }

  /**
   * Get tracks by kind
   */
  public getTracksByKind(kind: TrackKind): LocalTrack[] {
    return this.tracks.filter((t) => t.kind === kind);
  }

  /**
   * Clear all tracks
   */
  public clearTracks(): void {
    for (const track of this._tracks.values()) {
      track.stop();
    }
    this._tracks.clear();
  }
}

/** Subscribe callback type */
export type SubscribeCallback = (
  trackId: string,
  options: SubscribeOptions
) => Promise<{ subscriptionId: string }>;

/** Unsubscribe callback type */
export type UnsubscribeCallback = (subscriptionId: string) => Promise<void>;

/** Set layer callback type */
export type SetLayerCallback = (
  trackId: string,
  layer: SimulcastLayer
) => Promise<void>;

/**
 * Remote participant (other users)
 */
export class RemoteParticipant extends BaseParticipant<RemoteParticipantEvents> {
  private _tracks: Map<string, RemoteTrack> = new Map();
  private _connectionQuality: ConnectionQuality = 'good';
  private subscribeCallback?: SubscribeCallback;
  private unsubscribeCallback?: UnsubscribeCallback;
  private setLayerCallback?: SetLayerCallback;

  constructor(id: string, metadata: Record<string, unknown> = {}) {
    super(id, metadata);
  }

  /**
   * Get published tracks
   */
  public get tracks(): RemoteTrack[] {
    return Array.from(this._tracks.values());
  }

  /**
   * Get connection quality
   */
  public get connectionQuality(): ConnectionQuality {
    return this._connectionQuality;
  }

  /**
   * Set callbacks
   */
  public setCallbacks(callbacks: {
    subscribe?: SubscribeCallback;
    unsubscribe?: UnsubscribeCallback;
    setLayer?: SetLayerCallback;
  }): void {
    this.subscribeCallback = callbacks.subscribe;
    this.unsubscribeCallback = callbacks.unsubscribe;
    this.setLayerCallback = callbacks.setLayer;
  }

  /**
   * Add a track (called when trackPublished notification received)
   */
  public addTrack(
    id: string,
    kind: TrackKind,
    simulcast: boolean,
    metadata: Record<string, unknown> = {},
    name?: string
  ): RemoteTrack {
    const track = new RemoteTrack(id, kind, this.id, simulcast, metadata, name);

    // Set layer change callback
    if (this.setLayerCallback !== undefined) {
      const callback = this.setLayerCallback;
      track.setLayerChangeCallback(async (layer) => {
        await callback(id, layer);
      });
    }

    this._tracks.set(id, track);
    this.emit('trackAdded', track);
    return track;
  }

  /**
   * Remove a track (called when trackUnpublished notification received)
   */
  public removeTrack(trackId: string): RemoteTrack | undefined {
    const track = this._tracks.get(trackId);
    if (track !== undefined) {
      track.clearMediaStreamTrack();
      this._tracks.delete(trackId);
      this.emit('trackRemoved', track);
    }
    return track;
  }

  /**
   * Subscribe to a track
   */
  public async subscribe(
    trackId: string,
    options: SubscribeOptions = {}
  ): Promise<RemoteTrack> {
    const track = this._tracks.get(trackId);
    if (track === undefined) {
      throw new Error(`Track not found: ${trackId}`);
    }

    if (track.isSubscribed) {
      return track;
    }

    if (this.subscribeCallback === undefined) {
      throw new Error('Subscribe callback not set');
    }

    const result = await this.subscribeCallback(trackId, options);
    track.subscriptionId = result.subscriptionId;

    // Auto-attach if provided
    if (options.autoAttach !== undefined) {
      track.attach(options.autoAttach);
    }

    this.emit('trackSubscribed', track);
    return track;
  }

  /**
   * Unsubscribe from a track
   */
  public async unsubscribe(track: RemoteTrack): Promise<void> {
    if (track.subscriptionId === undefined) {
      throw new Error('Track is not subscribed');
    }

    if (this.unsubscribeCallback === undefined) {
      throw new Error('Unsubscribe callback not set');
    }

    await this.unsubscribeCallback(track.subscriptionId);
    track.clearMediaStreamTrack();
    this.emit('trackUnsubscribed', track);
  }

  /**
   * Get track by ID
   */
  public getTrack(trackId: string): RemoteTrack | undefined {
    return this._tracks.get(trackId);
  }

  /**
   * Get tracks by kind
   */
  public getTracksByKind(kind: TrackKind): RemoteTrack[] {
    return this.tracks.filter((t) => t.kind === kind);
  }

  /**
   * Update connection quality
   */
  public updateConnectionQuality(quality: ConnectionQuality): void {
    if (this._connectionQuality !== quality) {
      this._connectionQuality = quality;
      this.emit('connectionQualityChanged', quality);
    }
  }

  /**
   * Clear all tracks
   */
  public clearTracks(): void {
    for (const track of this._tracks.values()) {
      track.clearMediaStreamTrack();
    }
    this._tracks.clear();
  }
}
