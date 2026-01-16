/**
 * Local track management
 */

import { EventEmitter } from '../events/EventEmitter';
import type { TrackKind, PublishOptions } from '../utils/types';

/** Local track events */
interface LocalTrackEvents {
  muted: void;
  unmuted: void;
  ended: void;
}

/**
 * Represents a local media track that can be published
 */
export class LocalTrack extends EventEmitter<LocalTrackEvents> {
  /** Server-assigned track ID (set after publish) */
  public id: string | null = null;

  /** Track kind */
  public readonly kind: TrackKind;

  /** Track name/label */
  public readonly name: string | undefined;

  /** Browser MediaStreamTrack */
  public readonly mediaStreamTrack: MediaStreamTrack;

  /** Whether simulcast is enabled */
  public simulcast: boolean;

  /** Track metadata */
  public readonly metadata: Record<string, unknown>;

  /** Mute state */
  private _muted = false;

  constructor(
    mediaStreamTrack: MediaStreamTrack,
    options: PublishOptions = {}
  ) {
    super();
    this.mediaStreamTrack = mediaStreamTrack;
    this.kind = mediaStreamTrack.kind as TrackKind;
    this.name = options.name ?? mediaStreamTrack.label;
    this.simulcast = options.simulcast ?? (this.kind === 'video');
    this.metadata = options.metadata ?? {};

    // Listen for track ended
    this.mediaStreamTrack.addEventListener('ended', () => {
      this.emit('ended', undefined);
    });
  }

  /**
   * Get mute state
   */
  public get muted(): boolean {
    return this._muted;
  }

  /**
   * Mute the track
   */
  public mute(): void {
    if (!this._muted) {
      this._muted = true;
      this.mediaStreamTrack.enabled = false;
      this.emit('muted', undefined);
    }
  }

  /**
   * Unmute the track
   */
  public unmute(): void {
    if (this._muted) {
      this._muted = false;
      this.mediaStreamTrack.enabled = true;
      this.emit('unmuted', undefined);
    }
  }

  /**
   * Stop the track
   */
  public stop(): void {
    this.mediaStreamTrack.stop();
  }

  /**
   * Check if track is enabled
   */
  public get enabled(): boolean {
    return this.mediaStreamTrack.enabled;
  }

  /**
   * Set track enabled state
   */
  public set enabled(value: boolean) {
    this.mediaStreamTrack.enabled = value;
    if (value && this._muted) {
      this._muted = false;
      this.emit('unmuted', undefined);
    } else if (!value && !this._muted) {
      this._muted = true;
      this.emit('muted', undefined);
    }
  }

  /**
   * Get track settings
   */
  public getSettings(): MediaTrackSettings {
    return this.mediaStreamTrack.getSettings();
  }

  /**
   * Get track constraints
   */
  public getConstraints(): MediaTrackConstraints {
    return this.mediaStreamTrack.getConstraints();
  }

  /**
   * Apply constraints to the track
   */
  public async applyConstraints(
    constraints: MediaTrackConstraints
  ): Promise<void> {
    await this.mediaStreamTrack.applyConstraints(constraints);
  }
}
