/**
 * Core type definitions for the SFU Client SDK
 */

/** Track kind */
export type TrackKind = 'audio' | 'video';

/** Simulcast layer */
export type SimulcastLayer = 'h' | 'm' | 'l';

/** Connection state */
export type ConnectionState =
  | 'disconnected'
  | 'connecting'
  | 'connected'
  | 'reconnecting';

/** Room state (client-side) */
export type RoomState =
  | 'disconnected'
  | 'connecting'
  | 'joined'
  | 'reconnecting';

/** Server-side room state */
export type ServerRoomState =
  | 'created'
  | 'active'
  | 'locked'
  | 'closing'
  | 'closed';

/** Connection quality */
export type ConnectionQuality = 'excellent' | 'good' | 'fair' | 'poor';

/** Leave reason (for self) */
export type LeaveReason = 'voluntary' | 'timeout' | 'kicked';

/** Participant leave reason (for others) */
export type ParticipantLeaveReason = 'leave' | 'timeout' | 'kicked';

/** Disconnect reason */
export type DisconnectReason =
  | 'client_initiated'
  | 'server_shutdown'
  | 'room_closed'
  | 'kicked'
  | 'connection_error';

/** Reconnect reason */
export type ReconnectReason = 'ice_disconnected' | 'server_restart';

/** Layer change reason */
export type LayerChangeReason = 'bandwidth' | 'unavailable';

/** Client event names */
export type ClientEvent =
  | 'connecting'
  | 'connected'
  | 'disconnected'
  | 'reconnecting'
  | 'reconnected'
  | 'error';

/** Room event names */
export type RoomEvent =
  | 'stateChanged'
  | 'serverStateChanged'
  | 'participantJoined'
  | 'participantLeft'
  | 'trackPublished'
  | 'trackUnpublished'
  | 'trackSubscribed'
  | 'trackSubscriptionFailed'
  | 'layerChanged'
  | 'connectionQualityChanged'
  | 'reconnecting'
  | 'reconnected'
  | 'disconnected'
  | 'joined'
  | 'left'
  | 'error'
  | 'reconnectRequested'
  | 'recordingStarted'
  | 'recordingStopped';

/** Participant interface */
export interface Participant {
  id: string;
  metadata: Record<string, unknown>;
}

/** Simulcast layer changed event */
export interface LayerChangedEvent {
  trackId: string;
  requestedLayer: SimulcastLayer;
  actualLayer: SimulcastLayer;
  reason: LayerChangeReason;
}

/** Server error notification */
export interface ServerError {
  code: number;
  message: string;
  fatal: boolean;
}

/** Reconnect config */
export interface ReconnectConfig {
  maxAttempts: number;
  initialDelay: number;
  maxDelay: number;
  factor: number;
}

/** Logger config */
export interface LoggerConfig {
  level: 'error' | 'warn' | 'info' | 'debug';
  handler?: (level: string, message: string, data?: unknown) => void;
}

/** SFU Client config */
export interface SFUClientConfig {
  url: string;
  autoReconnect?: boolean;
  reconnect?: Partial<ReconnectConfig>;
  logger?: LoggerConfig;
  iceServers?: RTCIceServer[];
  e2ee?: E2EEConfig;
}

/** Join options */
export interface JoinOptions {
  sessionId?: string;
  metadata?: Record<string, unknown>;
  autoSubscribe?: boolean;
}

/** Publish options */
export interface PublishOptions {
  name?: string;
  simulcast?: boolean;
  metadata?: Record<string, unknown>;
  videoEncoding?: VideoEncodingOptions;
  audioEncoding?: AudioEncodingOptions;
}

/** Video encoding options */
export interface VideoEncodingOptions {
  maxBitrate?: number;
  maxFramerate?: number;
  priority?: 'low' | 'medium' | 'high';
}

/** Audio encoding options */
export interface AudioEncodingOptions {
  maxBitrate?: number;
  stereo?: boolean;
  dtx?: boolean;
}

/** Subscribe options */
export interface SubscribeOptions {
  preferredLayer?: SimulcastLayer;
  autoAttach?: HTMLMediaElement;
}

/** Track info from server */
export interface TrackInfo {
  id: string;
  kind: TrackKind;
  publisherId: string;
  name?: string;
  simulcast: boolean;
  metadata: Record<string, unknown>;
}

/** Participant info from server */
export interface ParticipantInfo {
  id: string;
  metadata: Record<string, unknown>;
  tracks: TrackInfo[];
}

/** Join response from server */
export interface JoinResponse {
  participantId: string;
  roomId: string;
  sessionId: string;
  iceServers: RTCIceServer[];
  participants: ParticipantInfo[];
  reconnected: boolean;
}

/** E2EE encryption algorithm */
export type E2EEAlgorithm = 'AES-GCM' | 'AES-CTR';

/** E2EE key ratchet strategy */
export type E2EERatchetStrategy = 'per-frame' | 'per-second' | 'manual';

/** E2EE configuration */
export interface E2EEConfig {
  /** Enable E2EE */
  enabled: boolean;
  /** Encryption algorithm (default: AES-GCM) */
  algorithm?: E2EEAlgorithm;
  /** Key ratchet strategy (default: manual) */
  ratchetStrategy?: E2EERatchetStrategy;
  /** Custom key provider */
  keyProvider?: E2EEKeyProvider;
}

/** E2EE key provider interface */
export interface E2EEKeyProvider {
  /** Get encryption key for a participant */
  getKey(participantId: string): Promise<CryptoKey | null>;
  /** Set encryption key for a participant */
  setKey(participantId: string, key: CryptoKey): Promise<void>;
  /** Remove encryption key for a participant */
  removeKey(participantId: string): Promise<void>;
  /** Ratchet key (generate new key from current) */
  ratchetKey(participantId: string): Promise<CryptoKey>;
}

/** E2EE frame metadata */
export interface E2EEFrameMetadata {
  /** Participant ID who sent this frame */
  participantId: string;
  /** Frame counter for replay protection */
  frameCounter: number;
  /** Key index for key rotation */
  keyIndex: number;
}
