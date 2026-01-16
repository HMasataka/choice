/**
 * useParticipants hook - manages participant list
 */

import { useState, useEffect, useCallback } from 'react';
import type { Room, RemoteParticipant } from '@sfu/client-sdk';

/** useParticipants return type */
export interface UseParticipantsReturn {
  /** List of participants */
  participants: RemoteParticipant[];
  /** Number of participants */
  participantCount: number;
  /** Get participant by ID */
  getParticipant: (id: string) => RemoteParticipant | undefined;
}

/**
 * Hook for managing participant list
 */
export function useParticipants(room: Room | null): UseParticipantsReturn {
  const [participants, setParticipants] = useState<RemoteParticipant[]>([]);

  useEffect(() => {
    if (room === null) {
      setParticipants([]);
      return;
    }

    // Initialize with current participants
    setParticipants([...room.participants]);

    const handleParticipantJoined = (participant: RemoteParticipant): void => {
      setParticipants((prev) => [...prev, participant]);
    };

    const handleParticipantLeft = ({
      participant,
    }: {
      participant: RemoteParticipant;
    }): void => {
      setParticipants((prev) => prev.filter((p) => p.id !== participant.id));
    };

    room.on('participantJoined', handleParticipantJoined);
    room.on('participantLeft', handleParticipantLeft);

    return () => {
      room.off('participantJoined', handleParticipantJoined);
      room.off('participantLeft', handleParticipantLeft);
    };
  }, [room]);

  const getParticipant = useCallback(
    (id: string): RemoteParticipant | undefined => {
      return participants.find((p) => p.id === id);
    },
    [participants]
  );

  return {
    participants,
    participantCount: participants.length,
    getParticipant,
  };
}
