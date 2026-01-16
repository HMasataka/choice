/**
 * useScreenShare hook - manages screen sharing
 */

import { useState, useCallback, useRef } from 'react';
import type { LocalParticipant, LocalTrack, PublishOptions } from '@sfu/client-sdk';

/** Screen share options */
export interface ScreenShareOptions extends PublishOptions {
  /** Display media constraints for video */
  video?: boolean | MediaTrackConstraints;
  /** Display media constraints for audio */
  audio?: boolean | MediaTrackConstraints;
}

/** useScreenShare return type */
export interface UseScreenShareReturn {
  /** Whether screen is being shared */
  isSharing: boolean;
  /** Screen share track */
  track: LocalTrack | null;
  /** Start screen sharing */
  startScreenShare: (options?: ScreenShareOptions) => Promise<void>;
  /** Stop screen sharing */
  stopScreenShare: () => Promise<void>;
  /** Error if any */
  error: Error | null;
}

/**
 * Hook for managing screen sharing
 */
export function useScreenShare(
  localParticipant: LocalParticipant | null
): UseScreenShareReturn {
  const [isSharing, setIsSharing] = useState(false);
  const [track, setTrack] = useState<LocalTrack | null>(null);
  const [error, setError] = useState<Error | null>(null);

  const streamRef = useRef<MediaStream | null>(null);
  const trackRef = useRef<LocalTrack | null>(null);

  const startScreenShare = useCallback(
    async (options: ScreenShareOptions = {}) => {
      if (localParticipant === null) {
        throw new Error('Local participant not available');
      }

      if (isSharing) {
        return;
      }

      setError(null);

      try {
        // Get display media
        const { video = true, audio = false, ...publishOptions } = options;

        const stream = await navigator.mediaDevices.getDisplayMedia({
          video,
          audio,
        } as DisplayMediaStreamOptions);

        streamRef.current = stream;

        // Get the video track
        const videoTrack = stream.getVideoTracks()[0];
        if (videoTrack === undefined) {
          throw new Error('No video track in screen share');
        }

        // Handle track ended (user clicked "Stop sharing" in browser UI)
        videoTrack.addEventListener('ended', () => {
          void stopScreenShare();
        });

        // Publish the track
        const localTrack = await localParticipant.publish(videoTrack, {
          name: 'screen',
          simulcast: true,
          metadata: { source: 'screen' },
          ...publishOptions,
        });

        trackRef.current = localTrack;
        setTrack(localTrack);
        setIsSharing(true);
      } catch (err) {
        if (err instanceof Error) {
          setError(err);
        }
        throw err;
      }
    },
    [localParticipant, isSharing]
  );

  const stopScreenShare = useCallback(async () => {
    if (!isSharing) {
      return;
    }

    try {
      // Unpublish the track
      if (trackRef.current !== null && localParticipant !== null) {
        await localParticipant.unpublish(trackRef.current);
        trackRef.current = null;
      }

      // Stop the stream
      if (streamRef.current !== null) {
        streamRef.current.getTracks().forEach((t) => t.stop());
        streamRef.current = null;
      }

      setTrack(null);
      setIsSharing(false);
    } catch (err) {
      if (err instanceof Error) {
        setError(err);
      }
      throw err;
    }
  }, [isSharing, localParticipant]);

  return {
    isSharing,
    track,
    startScreenShare,
    stopScreenShare,
    error,
  };
}
