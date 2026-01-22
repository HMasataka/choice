export type TrackKind = "audio" | "video";
export type SimulcastLayer = "h" | "m" | "l";
export type ConnectionState =
    | "disconnected"
    | "connecting"
    | "connected"
    | "reconnecting";
export type RoomState = "disconnected" | "connecting" | "joined" | "reconnecting";

export interface JoinResult {
    sessionId: string;
    participantId: string;
    roomId: string;
    iceServers: RTCIceServer[];
    participants: ParticipantInfo[];
    restored: boolean;
}

export interface ParticipantInfo {
    id: string;
    metadata: Record<string, unknown>;
    tracks: TrackInfo[];
}

export interface TrackInfo {
    trackId: string;
    kind: TrackKind;
    simulcast: boolean;
    metadata: Record<string, unknown>;
}

export interface PublishResult {
    trackId: string;
    mid: string;
}

export interface SubscribeResult {
    subscriptionId: string;
}

export interface LayerChangedEvent {
    trackId: string;
    requestedLayer: SimulcastLayer;
    actualLayer: SimulcastLayer;
    reason: "bandwidth" | "unavailable";
}

export interface ServerError {
    code: number;
    message: string;
    fatal: boolean;
}

export const ErrorCodes = {
    PARSE_ERROR: -32700,
    INVALID_REQUEST: -32600,
    METHOD_NOT_FOUND: -32601,
    INVALID_PARAMS: -32602,
    INTERNAL_ERROR: -32603,
    ROOM_NOT_FOUND: 1001,
    ROOM_FULL: 1002,
    UNAUTHORIZED: 1003,
    ALREADY_JOINED: 1004,
    NOT_IN_ROOM: 1005,
    TRACK_NOT_FOUND: 1006,
    INVALID_SDP: 1007,
    ICE_FAILURE: 1008,
    SESSION_EXPIRED: 1009,
} as const;
