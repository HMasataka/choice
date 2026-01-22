import { useState, useCallback, useEffect } from "react";

interface UseScreenShareReturn {
    stream: MediaStream | null;
    isSharing: boolean;
    error: Error | null;
    startScreenShare: () => Promise<void>;
    stopScreenShare: () => void;
}

export function useScreenShare(): UseScreenShareReturn {
    const [stream, setStream] = useState<MediaStream | null>(null);
    const [isSharing, setIsSharing] = useState(false);
    const [error, setError] = useState<Error | null>(null);

    const startScreenShare = useCallback(async () => {
        setError(null);

        try {
            const mediaStream = await navigator.mediaDevices.getDisplayMedia({
                video: true,
                audio: {
                    echoCancellation: false,
                    noiseSuppression: false,
                    autoGainControl: false,
                },
            });

            mediaStream.getVideoTracks()[0].onended = () => {
                setStream(null);
                setIsSharing(false);
            };

            setStream(mediaStream);
            setIsSharing(true);
        } catch (err) {
            if ((err as Error).name !== "NotAllowedError") {
                setError(err as Error);
            }
        }
    }, []);

    const stopScreenShare = useCallback(() => {
        if (stream) {
            stream.getTracks().forEach((track) => track.stop());
            setStream(null);
            setIsSharing(false);
        }
    }, [stream]);

    useEffect(() => {
        return () => {
            stream?.getTracks().forEach((track) => track.stop());
        };
    }, [stream]);

    return {
        stream,
        isSharing,
        error,
        startScreenShare,
        stopScreenShare,
    };
}
