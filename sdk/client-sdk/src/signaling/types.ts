/**
 * Signaling protocol type definitions (JSON-RPC 2.0)
 */

/** JSON-RPC request */
export interface JsonRpcRequest<T = unknown> {
  jsonrpc: '2.0';
  id: string;
  method: string;
  params?: T;
}

/** JSON-RPC response */
export interface JsonRpcResponse<T = unknown> {
  jsonrpc: '2.0';
  id: string;
  result?: T;
  error?: JsonRpcError;
}

/** JSON-RPC error */
export interface JsonRpcError {
  code: number;
  message: string;
  data?: unknown;
}

/** JSON-RPC notification (no id) */
export interface JsonRpcNotification<T = unknown> {
  jsonrpc: '2.0';
  method: string;
  params?: T;
}

/** Union type for all JSON-RPC messages */
export type JsonRpcMessage =
  | JsonRpcRequest
  | JsonRpcResponse
  | JsonRpcNotification;

// Request parameter types
export interface JoinParams {
  token: string;
  sessionId?: string;
  metadata?: Record<string, unknown>;
}

export interface LeaveParams {
  reason?: string;
}

export interface OfferParams {
  sdp: string;
}

export interface AnswerParams {
  sdp: string;
}

export interface CandidateParams {
  candidate: string;
  sdpMid?: string;
  sdpMLineIndex?: number;
}

export interface PublishParams {
  kind: 'audio' | 'video';
  simulcast?: boolean;
  metadata?: Record<string, unknown>;
  label?: string;
}

export interface UnpublishParams {
  trackId: string;
}

export interface SubscribeParams {
  publisherId: string;
  trackId: string;
  preferredLayer?: 'h' | 'm' | 'l';
}

export interface UnsubscribeParams {
  subscriptionId: string;
}

export interface SetPreferredLayerParams {
  trackId: string;
  layer: 'h' | 'm' | 'l';
}

// Response types
export interface JoinResult {
  participantId: string;
  roomId: string;
  sessionId: string;
  iceServers: RTCIceServer[];
  participants: ParticipantData[];
  reconnected: boolean;
}

export interface ParticipantData {
  id: string;
  metadata: Record<string, unknown>;
  tracks: TrackData[];
}

export interface TrackData {
  id: string;
  kind: 'audio' | 'video';
  publisherId: string;
  name?: string;
  simulcast: boolean;
  metadata: Record<string, unknown>;
}

export interface PublishResult {
  trackId: string;
}

export interface SubscribeResult {
  subscriptionId: string;
  trackId: string;
  publisherId: string;
}

// Notification parameter types
export interface ParticipantJoinedNotification {
  participant: ParticipantData;
}

export interface ParticipantLeftNotification {
  participantId: string;
  reason: 'leave' | 'timeout' | 'kicked';
}

export interface TrackPublishedNotification {
  publisherId: string;
  track: TrackData;
}

export interface TrackUnpublishedNotification {
  publisherId: string;
  trackId: string;
}

export interface TrackSubscribedNotification {
  subscriptionId: string;
  trackId: string;
  publisherId: string;
}

export interface TrackSubscriptionFailedNotification {
  trackId: string;
  error: JsonRpcError;
}

export interface LayerChangedNotification {
  trackId: string;
  requestedLayer: 'h' | 'm' | 'l';
  actualLayer: 'h' | 'm' | 'l';
  reason: 'bandwidth' | 'unavailable';
}

export interface ConnectionQualityChangedNotification {
  participantId: string;
  quality: 'excellent' | 'good' | 'fair' | 'poor';
}

export interface ServerStateChangedNotification {
  state: 'created' | 'active' | 'locked' | 'closing' | 'closed';
}

export interface ErrorNotification {
  code: number;
  message: string;
  fatal: boolean;
}

export interface ReconnectNotification {
  reason: 'ice_disconnected' | 'server_restart';
  retryAfterMs: number;
}

export interface OfferNotification {
  sdp: string;
}

export interface CandidateNotification {
  candidate: string;
  sdpMid?: string;
  sdpMLineIndex?: number;
}

export interface JoinedNotification {
  participantId: string;
  roomId: string;
}

export interface LeftNotification {
  reason: 'voluntary' | 'timeout' | 'kicked';
}

export interface RecordingStartedNotification {
  recordingId: string;
  startedBy: string;
}

export interface RecordingStoppedNotification {
  recordingId: string;
  stoppedBy: string;
}
