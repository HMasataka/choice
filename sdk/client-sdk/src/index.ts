/**
 * SFU Client SDK - Main Entry Point
 */

// Main client
export { SFUClient } from './client/SFUClient';

// Room and participants
export { Room } from './client/Room';
export type { RoomEvents } from './client/Room';
export { LocalParticipant, RemoteParticipant } from './client/Participant';
export { Connection } from './client/Connection';

// Media
export { LocalTrack } from './media/LocalTrack';
export { RemoteTrack } from './media/RemoteTrack';
export { MediaDevices } from './media/MediaDevices';
export type {
  DeviceInfo,
  MediaConstraints,
  ScreenShareOptions,
} from './media/MediaDevices';

// Simulcast
export {
  createSimulcastEncodings,
  getLayerConfig,
  getNextLowerLayer,
  getNextHigherLayer,
  compareLayers,
  getBestAvailableLayer,
  DEFAULT_SIMULCAST_LAYERS,
  SCREEN_SHARE_SIMULCAST_LAYERS,
} from './media/Simulcast';
export type { SimulcastLayerConfig } from './media/Simulcast';

// WebRTC utilities
export { PeerConnection } from './webrtc/PeerConnection';
export type { PeerConnectionOptions } from './webrtc/PeerConnection';
export { ICEManager } from './webrtc/ICEManager';
export type { ICEManagerOptions } from './webrtc/ICEManager';
export { SDPUtils } from './webrtc/SDPUtils';
export type { CodecInfo, MediaSection } from './webrtc/SDPUtils';

// Signaling
export { SignalingClient } from './signaling/SignalingClient';
export type {
  SignalingClientOptions,
  SignalingClientEvents,
} from './signaling/SignalingClient';
export { JsonRpcClient } from './signaling/JsonRpcClient';
export type { JsonRpcClientOptions } from './signaling/JsonRpcClient';

// Events
export { EventEmitter } from './events/EventEmitter';

// E2EE
export { E2EEManager, FrameCryptor, DefaultKeyProvider } from './e2ee';
export type { E2EEKeyProvider } from './e2ee';

// Errors
export { SFUError, ErrorCodes } from './errors/SFUError';
export type { ErrorCode } from './errors/SFUError';

// Utilities
export { Logger } from './utils/logger';
export {
  retry,
  sleep,
  calculateBackoffDelay,
  createRetryable,
  DEFAULT_RECONNECT_CONFIG,
} from './utils/retry';

// Types
export type {
  TrackKind,
  SimulcastLayer,
  ConnectionState,
  RoomState,
  ServerRoomState,
  ConnectionQuality,
  LeaveReason,
  ParticipantLeaveReason,
  DisconnectReason,
  ReconnectReason,
  LayerChangeReason,
  ClientEvent,
  RoomEvent,
  Participant,
  LayerChangedEvent,
  ServerError,
  ReconnectConfig,
  LoggerConfig,
  SFUClientConfig,
  JoinOptions,
  PublishOptions,
  VideoEncodingOptions,
  AudioEncodingOptions,
  SubscribeOptions,
  TrackInfo,
  ParticipantInfo,
  JoinResponse,
  E2EEConfig,
  E2EEAlgorithm,
  E2EERatchetStrategy,
  E2EEFrameMetadata,
} from './utils/types';

// Signaling types
export type {
  JsonRpcRequest,
  JsonRpcResponse,
  JsonRpcError,
  JsonRpcNotification,
  JoinParams,
  JoinResult,
  PublishParams,
  PublishResult,
  SubscribeParams,
  SubscribeResult,
  ParticipantData,
  TrackData,
} from './signaling/types';
