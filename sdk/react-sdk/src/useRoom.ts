/**
 * useRoom hook - manages room participation
 */

import { useState, useEffect, useCallback, useRef } from 'react';
import type {
  SFUClient,
  Room,
  RoomState,
  LocalParticipant,
  RemoteParticipant,
  JoinOptions,
} from '@sfu/client-sdk';

/** useRoom options */
export interface UseRoomOptions {
  /** SFU client instance */
  client: SFUClient | null;
  /** Join token */
  token: string;
  /** Auto-join on mount */
  autoJoin?: boolean;
  /** Join options */
  joinOptions?: Omit<JoinOptions, 'token'>;
}

/** useRoom return type */
export interface UseRoomReturn {
  /** Room instance */
  room: Room | null;
  /** Room state */
  state: RoomState;
  /** Remote participants */
  participants: RemoteParticipant[];
  /** Local participant */
  localParticipant: LocalParticipant | null;
  /** Join the room */
  join: () => Promise<void>;
  /** Leave the room */
  leave: () => Promise<void>;
  /** Error if any */
  error: Error | null;
}

/**
 * Hook for managing room participation
 */
export function useRoom(options: UseRoomOptions): UseRoomReturn {
  const { client, token, autoJoin, joinOptions } = options;

  const [room, setRoom] = useState<Room | null>(null);
  const [state, setState] = useState<RoomState>('disconnected');
  const [participants, setParticipants] = useState<RemoteParticipant[]>([]);
  const [localParticipant, setLocalParticipant] =
    useState<LocalParticipant | null>(null);
  const [error, setError] = useState<Error | null>(null);

  const roomRef = useRef<Room | null>(null);
  const mountedRef = useRef(true);

  // Set up room event handlers
  useEffect(() => {
    if (room === null) return;

    const handleStateChanged = (newState: RoomState): void => {
      if (mountedRef.current) {
        setState(newState);
      }
    };

    const handleParticipantJoined = (participant: RemoteParticipant): void => {
      if (mountedRef.current) {
        setParticipants((prev) => [...prev, participant]);
      }
    };

    const handleParticipantLeft = ({
      participant,
    }: {
      participant: RemoteParticipant;
    }): void => {
      if (mountedRef.current) {
        setParticipants((prev) => prev.filter((p) => p.id !== participant.id));
      }
    };

    room.on('stateChanged', handleStateChanged);
    room.on('participantJoined', handleParticipantJoined);
    room.on('participantLeft', handleParticipantLeft);

    return () => {
      room.off('stateChanged', handleStateChanged);
      room.off('participantJoined', handleParticipantJoined);
      room.off('participantLeft', handleParticipantLeft);
    };
  }, [room]);

  // Join function
  const join = useCallback(async () => {
    if (client === null) {
      throw new Error('Client not available');
    }

    setError(null);

    try {
      const newRoom = await client.join(token, joinOptions);

      if (mountedRef.current) {
        roomRef.current = newRoom;
        setRoom(newRoom);
        setState(newRoom.state);
        setParticipants([...newRoom.participants]);
        setLocalParticipant(newRoom.localParticipant);
      }
    } catch (err) {
      if (mountedRef.current && err instanceof Error) {
        setError(err);
      }
      throw err;
    }
  }, [client, token, joinOptions]);

  // Leave function
  const leave = useCallback(async () => {
    if (roomRef.current !== null) {
      await roomRef.current.leave();

      if (mountedRef.current) {
        setRoom(null);
        setState('disconnected');
        setParticipants([]);
        setLocalParticipant(null);
        roomRef.current = null;
      }
    }
  }, []);

  // Auto-join effect
  useEffect(() => {
    mountedRef.current = true;

    if (autoJoin === true && client !== null && token !== '') {
      void join().catch(() => {
        // Error is already set in join()
      });
    }

    return () => {
      mountedRef.current = false;
      if (roomRef.current !== null) {
        void roomRef.current.leave();
        roomRef.current = null;
      }
    };
  }, [autoJoin, client, token, join]);

  return {
    room,
    state,
    participants,
    localParticipant,
    join,
    leave,
    error,
  };
}
