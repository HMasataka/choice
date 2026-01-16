/**
 * useLocalMedia hook - manages local media devices
 */

import { useState, useEffect, useCallback, useRef } from 'react';

/** useLocalMedia options */
export interface UseLocalMediaOptions {
  /** Video constraints or false to disable */
  video?: boolean | MediaTrackConstraints;
  /** Audio constraints or false to disable */
  audio?: boolean | MediaTrackConstraints;
  /** Auto-start media on mount */
  autoStart?: boolean;
}

/** useLocalMedia return type */
export interface UseLocalMediaReturn {
  /** Media stream */
  stream: MediaStream | null;
  /** Video track */
  videoTrack: MediaStreamTrack | null;
  /** Audio track */
  audioTrack: MediaStreamTrack | null;
  /** Whether video is enabled */
  videoEnabled: boolean;
  /** Whether audio is enabled */
  audioEnabled: boolean;
  /** Whether media is loading */
  isLoading: boolean;
  /** Error if any */
  error: Error | null;
  /** Start capturing media */
  getMedia: () => Promise<void>;
  /** Stop all media tracks */
  stopMedia: () => void;
  /** Toggle video enabled state */
  toggleVideo: () => void;
  /** Toggle audio enabled state */
  toggleAudio: () => void;
}

/**
 * Hook for managing local media (camera/microphone)
 */
export function useLocalMedia(
  options: UseLocalMediaOptions = {}
): UseLocalMediaReturn {
  const { video = true, audio = true, autoStart = false } = options;

  const [stream, setStream] = useState<MediaStream | null>(null);
  const [videoTrack, setVideoTrack] = useState<MediaStreamTrack | null>(null);
  const [audioTrack, setAudioTrack] = useState<MediaStreamTrack | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  // Separate state for enabled status to trigger re-renders
  const [videoEnabled, setVideoEnabled] = useState(true);
  const [audioEnabled, setAudioEnabled] = useState(true);

  const mountedRef = useRef(true);
  const streamRef = useRef<MediaStream | null>(null);

  const getMedia = useCallback(async () => {
    setIsLoading(true);
    setError(null);

    try {
      const mediaStream = await navigator.mediaDevices.getUserMedia({
        video,
        audio,
      });

      if (!mountedRef.current) {
        // Component unmounted, stop tracks
        mediaStream.getTracks().forEach((track) => track.stop());
        return;
      }

      streamRef.current = mediaStream;
      setStream(mediaStream);

      const vTrack = mediaStream.getVideoTracks()[0];
      const aTrack = mediaStream.getAudioTracks()[0];

      setVideoTrack(vTrack ?? null);
      setAudioTrack(aTrack ?? null);
      // Sync enabled states with actual track states
      // If track doesn't exist, set enabled to false to reflect actual state
      setVideoEnabled(vTrack?.enabled ?? false);
      setAudioEnabled(aTrack?.enabled ?? false);
    } catch (err) {
      if (mountedRef.current && err instanceof Error) {
        setError(err);
      }
    } finally {
      if (mountedRef.current) {
        setIsLoading(false);
      }
    }
  }, [video, audio]);

  const stopMedia = useCallback(() => {
    if (streamRef.current !== null) {
      streamRef.current.getTracks().forEach((track) => track.stop());
      streamRef.current = null;
    }

    setStream(null);
    setVideoTrack(null);
    setAudioTrack(null);
    // Reset enabled states when stopping media
    setVideoEnabled(true);
    setAudioEnabled(true);
  }, []);

  const toggleVideo = useCallback(() => {
    if (videoTrack !== null) {
      const newEnabled = !videoTrack.enabled;
      videoTrack.enabled = newEnabled;
      setVideoEnabled(newEnabled);
    }
  }, [videoTrack]);

  const toggleAudio = useCallback(() => {
    if (audioTrack !== null) {
      const newEnabled = !audioTrack.enabled;
      audioTrack.enabled = newEnabled;
      setAudioEnabled(newEnabled);
    }
  }, [audioTrack]);

  // Auto-start effect
  useEffect(() => {
    mountedRef.current = true;

    if (autoStart) {
      void getMedia();
    }

    return () => {
      mountedRef.current = false;
      stopMedia();
    };
  }, [autoStart, getMedia, stopMedia]);

  return {
    stream,
    videoTrack,
    audioTrack,
    videoEnabled,
    audioEnabled,
    isLoading,
    error,
    getMedia,
    stopMedia,
    toggleVideo,
    toggleAudio,
  };
}
