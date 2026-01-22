import React, { useRef, useEffect } from "react";
import { UserIcon } from "./Icons";
import type { SimulcastLayer } from "@sfu/react-sdk";

interface VideoTileProps {
    stream: MediaStream | null;
    label: string;
    isLocal?: boolean;
    muted?: boolean;
    showLayerSelector?: boolean;
    currentLayer?: SimulcastLayer;
    onLayerChange?: (layer: SimulcastLayer) => void;
}

export const VideoTile: React.FC<VideoTileProps> = ({
    stream,
    label,
    isLocal = false,
    muted = false,
    showLayerSelector = false,
    currentLayer,
    onLayerChange,
}) => {
    const videoRef = useRef<HTMLVideoElement>(null);

    useEffect(() => {
        console.log(`VideoTile useEffect: label=${label}, hasStream=${!!stream}, hasRef=${!!videoRef.current}`);
        if (videoRef.current && stream) {
            // Debug: Check stream tracks
            const videoTracks = stream.getVideoTracks();
            const audioTracks = stream.getAudioTracks();
            console.log(`VideoTile: Setting srcObject for ${label}`, {
                streamId: stream.id,
                active: stream.active,
                videoTracks: videoTracks.map(t => ({
                    id: t.id,
                    enabled: t.enabled,
                    muted: t.muted,
                    readyState: t.readyState,
                    settings: t.getSettings(),
                })),
                audioTracks: audioTracks.length,
            });

            videoRef.current.srcObject = stream;

            // Handle autoplay - some browsers require user interaction
            videoRef.current.play().catch((err) => {
                console.warn(`VideoTile: autoplay failed for ${label}:`, err);
            });

            // Debug: Log video dimensions when available
            const video = videoRef.current;
            const checkDimensions = () => {
                console.log(`VideoTile: video dimensions for ${label}:`, {
                    videoWidth: video.videoWidth,
                    videoHeight: video.videoHeight,
                    readyState: video.readyState,
                    networkState: video.networkState,
                });
            };

            // Check dimensions periodically
            const intervalId = setInterval(checkDimensions, 2000);
            setTimeout(checkDimensions, 500);

            return () => clearInterval(intervalId);
        }
    }, [stream, label]);

    return (
        <div className={`video-container ${isLocal ? "local" : ""}`}>
            {stream ? (
                <video
                    ref={videoRef}
                    autoPlay
                    playsInline
                    muted={muted || isLocal}
                    style={isLocal ? { transform: "scaleX(-1)" } : undefined}
                    onLoadedMetadata={() => console.log(`VideoTile: video loaded metadata for ${label}`)}
                    onPlay={() => console.log(`VideoTile: video playing for ${label}`)}
                    onError={(e) => console.error(`VideoTile: video error for ${label}:`, e)}
                />
            ) : (
                <div className="video-placeholder">
                    <UserIcon />
                </div>
            )}
            <div className="video-label">
                {label}
                {showLayerSelector && onLayerChange && (
                    <div className="layer-selector">
                        {(["h", "m", "l"] as SimulcastLayer[]).map((layer) => (
                            <button
                                key={layer}
                                className={`layer-btn ${currentLayer === layer ? "active" : ""}`}
                                onClick={() => onLayerChange(layer)}
                            >
                                {layer === "h" ? "HD" : layer === "m" ? "SD" : "LD"}
                            </button>
                        ))}
                    </div>
                )}
            </div>
        </div>
    );
};
