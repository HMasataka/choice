/**
 * Connection management wrapper
 */

import { EventEmitter } from '../events/EventEmitter';
import { SignalingClient } from '../signaling/SignalingClient';
import type { ConnectionState } from '../utils/types';

/** Connection events */
interface ConnectionEvents {
  connecting: void;
  connected: void;
  disconnected: void;
  error: Error;
}

/**
 * Connection wrapper for signaling transport
 */
export class Connection extends EventEmitter<ConnectionEvents> {
  private readonly signaling: SignalingClient;
  private state: ConnectionState = 'disconnected';

  constructor(signaling: SignalingClient) {
    super();
    this.signaling = signaling;

    this.signaling.on('connected', () => {
      this.state = 'connected';
      this.emit('connected', undefined);
    });

    this.signaling.on('disconnected', () => {
      this.state = 'disconnected';
      this.emit('disconnected', undefined);
    });

    this.signaling.on('error', (err) => {
      this.emit('error', err as unknown as Error);
    });
  }

  /**
   * Connect to signaling
   */
  public async connect(url: string): Promise<void> {
    this.state = 'connecting';
    this.emit('connecting', undefined);
    await this.signaling.connect(url);
  }

  /**
   * Disconnect from signaling
   */
  public disconnect(): void {
    this.signaling.disconnect();
  }

  /**
   * Get connection state
   */
  public get connectionState(): ConnectionState {
    return this.state;
  }
}
