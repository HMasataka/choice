/**
 * Signaling client for SFU communication
 */

import { EventEmitter } from '../events/EventEmitter';
import { JsonRpcClient } from './JsonRpcClient';
import { Logger } from '../utils/logger';
import type { LoggerConfig } from '../utils/types';
import type {
  JoinParams,
  JoinResult,
  LeaveParams,
  OfferParams,
  AnswerParams,
  CandidateParams,
  PublishParams,
  PublishResult,
  UnpublishParams,
  SubscribeParams,
  SubscribeResult,
  UnsubscribeParams,
  SetPreferredLayerParams,
  JsonRpcNotification,
  ParticipantJoinedNotification,
  ParticipantLeftNotification,
  TrackPublishedNotification,
  TrackUnpublishedNotification,
  TrackSubscribedNotification,
  TrackSubscriptionFailedNotification,
  LayerChangedNotification,
  ConnectionQualityChangedNotification,
  ServerStateChangedNotification,
  ErrorNotification,
  ReconnectNotification,
  OfferNotification,
  CandidateNotification,
  JoinedNotification,
  LeftNotification,
  RecordingStartedNotification,
  RecordingStoppedNotification,
} from './types';

/** Signaling client events */
export interface SignalingClientEvents {
  // Connection events
  connected: void;
  disconnected: CloseEvent;
  error: Event;

  // Server notifications
  joined: JoinedNotification;
  left: LeftNotification;
  participantJoined: ParticipantJoinedNotification;
  participantLeft: ParticipantLeftNotification;
  trackPublished: TrackPublishedNotification;
  trackUnpublished: TrackUnpublishedNotification;
  trackSubscribed: TrackSubscribedNotification;
  trackSubscriptionFailed: TrackSubscriptionFailedNotification;
  layerChanged: LayerChangedNotification;
  connectionQualityChanged: ConnectionQualityChangedNotification;
  serverStateChanged: ServerStateChangedNotification;
  serverError: ErrorNotification;
  reconnectRequested: ReconnectNotification;
  offer: OfferNotification;
  candidate: CandidateNotification;
  recordingStarted: RecordingStartedNotification;
  recordingStopped: RecordingStoppedNotification;
}

/** Signaling client options */
export interface SignalingClientOptions {
  requestTimeout?: number;
  logger?: LoggerConfig;
}

/**
 * Signaling client for WebRTC SFU
 */
export class SignalingClient extends EventEmitter<SignalingClientEvents> {
  private readonly rpc: JsonRpcClient;
  private readonly logger: Logger;
  private connected = false;

  constructor(options: SignalingClientOptions = {}) {
    super();
    this.rpc = new JsonRpcClient({
      requestTimeout: options.requestTimeout,
      logger: options.logger,
    });
    this.logger = new Logger(options.logger, 'Signaling');

    this.setupRpcHandlers();
  }

  /**
   * Set up JSON-RPC event handlers
   */
  private setupRpcHandlers(): void {
    this.rpc.on('open', () => {
      this.connected = true;
      this.emit('connected', undefined);
    });

    this.rpc.on('close', (event) => {
      this.connected = false;
      this.emit('disconnected', event);
    });

    this.rpc.on('error', (event) => {
      this.emit('error', event);
    });

    this.rpc.on('notification', (notification) => {
      this.handleNotification(notification);
    });
  }

  /**
   * Handle incoming notifications
   */
  private handleNotification(notification: JsonRpcNotification): void {
    const { method, params } = notification;

    switch (method) {
      case 'joined':
        this.emit('joined', params as JoinedNotification);
        break;
      case 'left':
        this.emit('left', params as LeftNotification);
        break;
      case 'participantJoined':
        this.emit('participantJoined', params as ParticipantJoinedNotification);
        break;
      case 'participantLeft':
        this.emit('participantLeft', params as ParticipantLeftNotification);
        break;
      case 'trackPublished':
        this.emit('trackPublished', params as TrackPublishedNotification);
        break;
      case 'trackUnpublished':
        this.emit('trackUnpublished', params as TrackUnpublishedNotification);
        break;
      case 'trackSubscribed':
        this.emit('trackSubscribed', params as TrackSubscribedNotification);
        break;
      case 'trackSubscriptionFailed':
        this.emit('trackSubscriptionFailed', params as TrackSubscriptionFailedNotification);
        break;
      case 'layerChanged':
        this.emit('layerChanged', params as LayerChangedNotification);
        break;
      case 'connectionQualityChanged':
        this.emit('connectionQualityChanged', params as ConnectionQualityChangedNotification);
        break;
      case 'serverStateChanged':
        this.emit('serverStateChanged', params as ServerStateChangedNotification);
        break;
      case 'error':
        this.emit('serverError', params as ErrorNotification);
        break;
      case 'reconnect':
        this.emit('reconnectRequested', params as ReconnectNotification);
        break;
      case 'offer':
        this.emit('offer', params as OfferNotification);
        break;
      case 'candidate':
        this.emit('candidate', params as CandidateNotification);
        break;
      case 'recordingStarted':
        this.emit('recordingStarted', params as RecordingStartedNotification);
        break;
      case 'recordingStopped':
        this.emit('recordingStopped', params as RecordingStoppedNotification);
        break;
      default:
        this.logger.warn(`Unknown notification: ${method}`, params);
    }
  }

  /**
   * Connect to signaling server
   */
  public async connect(url: string): Promise<void> {
    this.logger.info(`Connecting to ${url}`);
    await this.rpc.connect(url);
  }

  /**
   * Disconnect from signaling server
   */
  public disconnect(): void {
    this.logger.info('Disconnecting');
    this.rpc.disconnect();
    this.connected = false;
  }

  /**
   * Close the signaling connection (alias for disconnect)
   */
  public close(): void {
    this.disconnect();
  }

  /**
   * Check if connected
   */
  public isConnected(): boolean {
    return this.connected && this.rpc.isConnected();
  }

  /**
   * Send a raw JSON-RPC request
   */
  public async send<TParams, TResult>(
    method: string,
    params?: TParams
  ): Promise<TResult> {
    return this.rpc.request<TParams, TResult>(method, params);
  }

  // ---- Signaling methods ----

  /**
   * Join a room
   */
  public async join(params: JoinParams): Promise<JoinResult> {
    this.logger.debug('Joining room', { hasSessionId: params.sessionId !== undefined });
    return this.rpc.request<JoinParams, JoinResult>('join', params);
  }

  /**
   * Leave a room
   */
  public async leave(params?: LeaveParams): Promise<void> {
    this.logger.debug('Leaving room');
    await this.rpc.request<LeaveParams | undefined, void>('leave', params);
  }

  /**
   * Send SDP offer
   */
  public async offer(params: OfferParams): Promise<{ sdp: string }> {
    this.logger.debug('Sending offer');
    return this.rpc.request<OfferParams, { sdp: string }>('offer', params);
  }

  /**
   * Send SDP answer
   */
  public async answer(params: AnswerParams): Promise<void> {
    this.logger.debug('Sending answer');
    await this.rpc.request<AnswerParams, void>('answer', params);
  }

  /**
   * Send ICE candidate
   */
  public async candidate(params: CandidateParams): Promise<void> {
    this.logger.debug('Sending candidate');
    await this.rpc.request<CandidateParams, void>('candidate', params);
  }

  /**
   * Publish a track
   */
  public async publish(params: PublishParams): Promise<PublishResult> {
    this.logger.debug('Publishing track', { kind: params.kind });
    return this.rpc.request<PublishParams, PublishResult>('publish', params);
  }

  /**
   * Unpublish a track
   */
  public async unpublish(params: UnpublishParams): Promise<void> {
    this.logger.debug('Unpublishing track', { trackId: params.trackId });
    await this.rpc.request<UnpublishParams, void>('unpublish', params);
  }

  /**
   * Subscribe to a track
   */
  public async subscribe(params: SubscribeParams): Promise<SubscribeResult> {
    this.logger.debug('Subscribing to track', { trackId: params.trackId });
    return this.rpc.request<SubscribeParams, SubscribeResult>('subscribe', params);
  }

  /**
   * Unsubscribe from a track
   */
  public async unsubscribe(params: UnsubscribeParams): Promise<void> {
    this.logger.debug('Unsubscribing', { subscriptionId: params.subscriptionId });
    await this.rpc.request<UnsubscribeParams, void>('unsubscribe', params);
  }

  /**
   * Set preferred simulcast layer
   */
  public async setPreferredLayer(params: SetPreferredLayerParams): Promise<void> {
    this.logger.debug('Setting preferred layer', params);
    await this.rpc.request<SetPreferredLayerParams, void>('setPreferredLayer', params);
  }
}
