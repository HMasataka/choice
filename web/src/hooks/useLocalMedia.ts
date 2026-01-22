import { useState, useCallback, useEffect, useRef } from "react";

interface UseLocalMediaOptions {
    video?: boolean | MediaTrackConstraints;
    audio?: boolean | MediaTrackConstraints;
}

interface UseLocalMediaReturn {
    stream: MediaStream | null;
    videoTrack: MediaStreamTrack | null;
    audioTrack: MediaStreamTrack | null;
    isLoading: boolean;
    error: Error | null;
    getMedia: () => Promise<void>;
    stopMedia: () => void;
    toggleVideo: () => void;
    toggleAudio: () => void;
    isVideoEnabled: boolean;
    isAudioEnabled: boolean;
}

export function useLocalMedia(
    options: UseLocalMediaOptions = {},
): UseLocalMediaReturn {
    const [stream, setStream] = useState<MediaStream | null>(null);
    const [isLoading, setIsLoading] = useState(false);
    const [error, setError] = useState<Error | null>(null);
    const [isVideoEnabled, setIsVideoEnabled] = useState(true);
    const [isAudioEnabled, setIsAudioEnabled] = useState(true);
    const optionsRef = useRef(options);
    // Use ref to track stream for cleanup - prevents stopping tracks on re-renders
    const streamRef = useRef<MediaStream | null>(null);

    useEffect(() => {
        optionsRef.current = options;
    }, [options]);

    const getMedia = useCallback(async () => {
        setIsLoading(true);
        setError(null);

        try {
            const constraints: MediaStreamConstraints = {
                video: optionsRef.current.video ?? {
                    width: { ideal: 1280 },
                    height: { ideal: 720 },
                    frameRate: { ideal: 30 },
                },
                audio: optionsRef.current.audio ?? {
                    echoCancellation: true,
                    noiseSuppression: true,
                    autoGainControl: true,
                },
            };

            const mediaStream =
                await navigator.mediaDevices.getUserMedia(constraints);
            streamRef.current = mediaStream;
            setStream(mediaStream);
            setIsVideoEnabled(true);
            setIsAudioEnabled(true);
        } catch (err) {
            setError(err as Error);
        } finally {
            setIsLoading(false);
        }
    }, []);

    const stopMedia = useCallback(() => {
        if (streamRef.current) {
            streamRef.current.getTracks().forEach((track) => track.stop());
            streamRef.current = null;
            setStream(null);
        }
    }, []);

    const toggleVideo = useCallback(() => {
        if (streamRef.current) {
            const videoTrack = streamRef.current.getVideoTracks()[0];
            if (videoTrack) {
                videoTrack.enabled = !videoTrack.enabled;
                setIsVideoEnabled(videoTrack.enabled);
            }
        }
    }, []);

    const toggleAudio = useCallback(() => {
        if (streamRef.current) {
            const audioTrack = streamRef.current.getAudioTracks()[0];
            if (audioTrack) {
                audioTrack.enabled = !audioTrack.enabled;
                setIsAudioEnabled(audioTrack.enabled);
            }
        }
    }, []);

    // Only stop tracks on component unmount, not on every stream change
    useEffect(() => {
        return () => {
            streamRef.current?.getTracks().forEach((track) => track.stop());
        };
    }, []);

    return {
        stream,
        videoTrack: stream?.getVideoTracks()[0] ?? null,
        audioTrack: stream?.getAudioTracks()[0] ?? null,
        isLoading,
        error,
        getMedia,
        stopMedia,
        toggleVideo,
        toggleAudio,
        isVideoEnabled,
        isAudioEnabled,
    };
}
