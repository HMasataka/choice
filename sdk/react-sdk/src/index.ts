/**
 * React SDK for WebRTC SFU
 */

export { useSFUClient } from './useSFUClient';
export type { UseSFUClientOptions, UseSFUClientReturn } from './useSFUClient';

export { useRoom } from './useRoom';
export type { UseRoomOptions, UseRoomReturn } from './useRoom';

export { useLocalMedia } from './useLocalMedia';
export type { UseLocalMediaOptions, UseLocalMediaReturn } from './useLocalMedia';

export { useRemoteTrack } from './useRemoteTrack';
export type {
  UseRemoteTrackOptions,
  UseRemoteTrackReturn,
} from './useRemoteTrack';

export { useParticipants } from './useParticipants';
export type { UseParticipantsReturn } from './useParticipants';

export { useScreenShare } from './useScreenShare';
export type { ScreenShareOptions, UseScreenShareReturn } from './useScreenShare';

// Re-export types from client-sdk for convenience
export type {
  SFUClient,
  Room,
  LocalParticipant,
  RemoteParticipant,
  LocalTrack,
  RemoteTrack,
  ConnectionState,
  RoomState,
  SimulcastLayer,
  ConnectionQuality,
} from '@sfu/client-sdk';
