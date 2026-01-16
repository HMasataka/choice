/**
 * Remote track management
 */

import { EventEmitter } from '../events/EventEmitter';
import type { TrackKind, SimulcastLayer } from '../utils/types';

/** Remote track events */
interface RemoteTrackEvents {
  subscribed: MediaStreamTrack;
  unsubscribed: void;
  layerChanged: SimulcastLayer;
}

/**
 * Represents a remote media track from another participant
 */
export class RemoteTrack extends EventEmitter<RemoteTrackEvents> {
  /** Track ID */
  public readonly id: string;

  /** Track kind */
  public readonly kind: TrackKind;

  /** Publisher ID */
  public readonly publisherId: string;

  /** Subscription ID (set after subscribe) */
  public subscriptionId: string | undefined;

  /** Browser MediaStreamTrack (set after subscribe) */
  private _mediaStreamTrack: MediaStreamTrack | null = null;

  /** Whether simulcast is enabled */
  public readonly simulcast: boolean;

  /** Current simulcast layer */
  private _currentLayer: SimulcastLayer | null = null;

  /** Track metadata */
  public readonly metadata: Record<string, unknown>;

  /** Track name/label */
  public readonly name: string | undefined;

  /** Attached media element */
  private attachedElement: HTMLMediaElement | null = null;

  /** Callback to set preferred layer */
  private setLayerCallback?: (layer: SimulcastLayer) => Promise<void>;

  constructor(
    id: string,
    kind: TrackKind,
    publisherId: string,
    simulcast: boolean,
    metadata: Record<string, unknown> = {},
    name?: string
  ) {
    super();
    this.id = id;
    this.kind = kind;
    this.publisherId = publisherId;
    this.simulcast = simulcast;
    this.metadata = metadata;
    this.name = name;
  }

  /**
   * Get MediaStreamTrack
   */
  public get mediaStreamTrack(): MediaStreamTrack | null {
    return this._mediaStreamTrack;
  }

  /**
   * Get current simulcast layer
   */
  public get currentLayer(): SimulcastLayer | null {
    return this._currentLayer;
  }

  /**
   * Check if track is subscribed
   */
  public get isSubscribed(): boolean {
    return this._mediaStreamTrack !== null;
  }

  /**
   * Set the MediaStreamTrack (called internally when subscribed)
   */
  public setMediaStreamTrack(track: MediaStreamTrack): void {
    this._mediaStreamTrack = track;
    this.emit('subscribed', track);

    // Auto-attach if element is set
    if (this.attachedElement !== null) {
      this.attachToElement(this.attachedElement);
    }
  }

  /**
   * Clear the MediaStreamTrack (called internally when unsubscribed)
   */
  public clearMediaStreamTrack(): void {
    if (this.attachedElement !== null) {
      this.detach();
    }
    this._mediaStreamTrack = null;
    this.subscriptionId = undefined;
    this.emit('unsubscribed', undefined);
  }

  /**
   * Update current layer (called internally)
   */
  public updateCurrentLayer(layer: SimulcastLayer): void {
    if (this._currentLayer !== layer) {
      this._currentLayer = layer;
      this.emit('layerChanged', layer);
    }
  }

  /**
   * Set the callback for setting preferred layer
   */
  public setLayerChangeCallback(
    callback: (layer: SimulcastLayer) => Promise<void>
  ): void {
    this.setLayerCallback = callback;
  }

  /**
   * Attach track to a media element
   */
  public attach(element: HTMLMediaElement): void {
    // Detach from existing element if already attached
    if (this.attachedElement !== null && this.attachedElement !== element) {
      this.attachedElement.srcObject = null;
    }

    this.attachedElement = element;

    if (this._mediaStreamTrack !== null) {
      this.attachToElement(element);
    }
  }

  /**
   * Detach track from media element
   */
  public detach(): void {
    if (this.attachedElement !== null) {
      this.attachedElement.srcObject = null;
      this.attachedElement = null;
    }
  }

  /**
   * Set preferred simulcast layer
   */
  public async setPreferredLayer(layer: SimulcastLayer): Promise<void> {
    if (!this.simulcast) {
      throw new Error('Track does not support simulcast');
    }

    if (this.setLayerCallback !== undefined) {
      await this.setLayerCallback(layer);
    }
  }

  /**
   * Internal method to attach track to element
   */
  private attachToElement(element: HTMLMediaElement): void {
    if (this._mediaStreamTrack === null) {
      return;
    }

    const stream = new MediaStream([this._mediaStreamTrack]);
    element.srcObject = stream;

    // Auto-play handling
    element.play().catch((error: unknown) => {
      // Handle autoplay policy - user interaction may be required
      console.warn('Autoplay failed:', error);
    });
  }
}
