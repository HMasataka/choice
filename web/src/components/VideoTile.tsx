import React, { useRef, useEffect } from "react";
import { UserIcon } from "./Icons";
import { SimulcastLayer } from "../sfu";

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
            console.log(`VideoTile: Setting srcObject for ${label}, streamId=${stream.id}, active=${stream.active}`);
            videoRef.current.srcObject = stream;

            // Handle autoplay - some browsers require user interaction
            videoRef.current.play().catch((err) => {
                console.warn(`VideoTile: autoplay failed for ${label}:`, err);
            });
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
