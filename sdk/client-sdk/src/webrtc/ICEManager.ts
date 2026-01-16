/**
 * ICE connection management
 */

import { EventEmitter } from '../events/EventEmitter';
import { Logger } from '../utils/logger';
import type { LoggerConfig } from '../utils/types';

/** ICE manager events */
interface ICEManagerEvents {
  candidate: RTCIceCandidate;
  connectionStateChange: RTCIceConnectionState;
  gatheringStateChange: RTCIceGatheringState;
  iceRestart: void;
}

/** ICE manager options */
export interface ICEManagerOptions {
  iceServers?: RTCIceServer[];
  iceCandidatePoolSize?: number;
  logger?: LoggerConfig;
}

/**
 * ICE connection manager
 */
export class ICEManager extends EventEmitter<ICEManagerEvents> {
  private readonly logger: Logger;
  private iceServers: RTCIceServer[];
  private readonly iceCandidatePoolSize: number;
  private connectionState: RTCIceConnectionState = 'new';
  private gatheringState: RTCIceGatheringState = 'new';
  private restartCount = 0;

  constructor(options: ICEManagerOptions = {}) {
    super();
    this.iceServers = options.iceServers ?? [];
    this.iceCandidatePoolSize = options.iceCandidatePoolSize ?? 0;
    this.logger = new Logger(options.logger, 'ICE');
  }

  /**
   * Get RTCConfiguration for peer connection
   */
  public getConfiguration(): RTCConfiguration {
    return {
      iceServers: this.iceServers,
      iceCandidatePoolSize: this.iceCandidatePoolSize,
      iceTransportPolicy: 'all',
      bundlePolicy: 'max-bundle',
      rtcpMuxPolicy: 'require',
    };
  }

  /**
   * Update ICE servers
   */
  public setIceServers(servers: RTCIceServer[]): void {
    this.iceServers = servers;
    this.logger.info('ICE servers updated', { count: servers.length });
  }

  /**
   * Handle ICE candidate from peer connection
   */
  public handleIceCandidate(candidate: RTCIceCandidate | null): void {
    if (candidate !== null) {
      this.logger.debug('ICE candidate generated', {
        type: candidate.type,
        protocol: candidate.protocol,
      });
      this.emit('candidate', candidate);
    } else {
      this.logger.debug('ICE gathering complete');
    }
  }

  /**
   * Handle ICE connection state change
   */
  public handleConnectionStateChange(state: RTCIceConnectionState): void {
    this.logger.info('ICE connection state changed', {
      from: this.connectionState,
      to: state,
    });
    this.connectionState = state;
    this.emit('connectionStateChange', state);
  }

  /**
   * Handle ICE gathering state change
   */
  public handleGatheringStateChange(state: RTCIceGatheringState): void {
    this.logger.debug('ICE gathering state changed', {
      from: this.gatheringState,
      to: state,
    });
    this.gatheringState = state;
    this.emit('gatheringStateChange', state);
  }

  /**
   * Get current connection state
   */
  public getConnectionState(): RTCIceConnectionState {
    return this.connectionState;
  }

  /**
   * Get current gathering state
   */
  public getGatheringState(): RTCIceGatheringState {
    return this.gatheringState;
  }

  /**
   * Check if ICE restart is needed
   */
  public needsRestart(): boolean {
    return (
      this.connectionState === 'failed' ||
      this.connectionState === 'disconnected'
    );
  }

  /**
   * Trigger ICE restart
   */
  public triggerRestart(): void {
    this.restartCount++;
    this.logger.info('ICE restart triggered', { count: this.restartCount });
    this.emit('iceRestart', undefined);
  }

  /**
   * Get restart count
   */
  public getRestartCount(): number {
    return this.restartCount;
  }

  /**
   * Reset manager state
   */
  public reset(): void {
    this.connectionState = 'new';
    this.gatheringState = 'new';
    this.restartCount = 0;
    this.logger.debug('ICE manager reset');
  }

  /**
   * Parse ICE candidate string to extract info
   */
  public static parseCandidate(candidateStr: string): {
    foundation: string;
    component: number;
    protocol: string;
    priority: number;
    ip: string;
    port: number;
    type: string;
  } | null {
    // candidate:123456 1 udp 12345678 192.168.1.1 5000 typ host
    const match =
      /candidate:(\S+)\s+(\d+)\s+(\S+)\s+(\d+)\s+(\S+)\s+(\d+)\s+typ\s+(\S+)/.exec(
        candidateStr
      );

    if (match === null) {
      return null;
    }

    return {
      foundation: match[1] ?? '',
      component: parseInt(match[2] ?? '0', 10),
      protocol: match[3] ?? '',
      priority: parseInt(match[4] ?? '0', 10),
      ip: match[5] ?? '',
      port: parseInt(match[6] ?? '0', 10),
      type: match[7] ?? '',
    };
  }

  /**
   * Check if candidate is a host candidate
   */
  public static isHostCandidate(candidate: RTCIceCandidate): boolean {
    return candidate.type === 'host';
  }

  /**
   * Check if candidate is a server reflexive candidate
   */
  public static isSrflxCandidate(candidate: RTCIceCandidate): boolean {
    return candidate.type === 'srflx';
  }

  /**
   * Check if candidate is a relay candidate
   */
  public static isRelayCandidate(candidate: RTCIceCandidate): boolean {
    return candidate.type === 'relay';
  }
}
