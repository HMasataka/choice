/**
 * useRemoteTrack hook - manages remote track subscription and playback
 */

import type { RefObject } from 'react';
import { useState, useEffect, useCallback, useRef } from 'react';
import type { RemoteTrack, SimulcastLayer } from '@sfu/client-sdk';

/** useRemoteTrack options */
export interface UseRemoteTrackOptions {
  /** Remote track to manage */
  track: RemoteTrack;
  /** Preferred simulcast layer */
  preferredLayer?: SimulcastLayer;
}

/** useRemoteTrack return type */
export interface UseRemoteTrackReturn {
  /** Ref for the media element */
  mediaRef: RefObject<HTMLVideoElement | HTMLAudioElement>;
  /** Whether track is subscribed */
  isSubscribed: boolean;
  /** Current simulcast layer */
  currentLayer: SimulcastLayer | null;
  /** Set preferred layer */
  setPreferredLayer: (layer: SimulcastLayer) => Promise<void>;
}

/**
 * Hook for managing remote track playback
 */
export function useRemoteTrack(
  options: UseRemoteTrackOptions
): UseRemoteTrackReturn {
  const { track, preferredLayer } = options;

  const [isSubscribed, setIsSubscribed] = useState(track.isSubscribed);
  const [currentLayer, setCurrentLayer] = useState<SimulcastLayer | null>(
    track.currentLayer
  );

  const mediaRef = useRef<HTMLVideoElement | HTMLAudioElement>(null);

  // Attach track to media element when subscribed
  useEffect(() => {
    const handleSubscribed = (mediaStreamTrack: MediaStreamTrack): void => {
      setIsSubscribed(true);

      if (mediaRef.current !== null) {
        const stream = new MediaStream([mediaStreamTrack]);
        mediaRef.current.srcObject = stream;
        void mediaRef.current.play().catch(() => {
          // Autoplay may be blocked
        });
      }
    };

    const handleUnsubscribed = (): void => {
      setIsSubscribed(false);
      if (mediaRef.current !== null) {
        mediaRef.current.srcObject = null;
      }
    };

    const handleLayerChanged = (layer: SimulcastLayer): void => {
      setCurrentLayer(layer);
    };

    track.on('subscribed', handleSubscribed);
    track.on('unsubscribed', handleUnsubscribed);
    track.on('layerChanged', handleLayerChanged);

    // If already subscribed, attach
    if (track.isSubscribed && track.mediaStreamTrack !== null) {
      handleSubscribed(track.mediaStreamTrack);
    }

    return () => {
      track.off('subscribed', handleSubscribed);
      track.off('unsubscribed', handleUnsubscribed);
      track.off('layerChanged', handleLayerChanged);
    };
  }, [track]);

  // Set initial preferred layer
  useEffect(() => {
    if (preferredLayer !== undefined && track.simulcast) {
      void track.setPreferredLayer(preferredLayer).catch(() => {
        // Layer may not be available
      });
    }
  }, [track, preferredLayer]);

  const setPreferredLayerCallback = useCallback(
    async (layer: SimulcastLayer) => {
      await track.setPreferredLayer(layer);
    },
    [track]
  );

  return {
    mediaRef,
    isSubscribed,
    currentLayer,
    setPreferredLayer: setPreferredLayerCallback,
  };
}
