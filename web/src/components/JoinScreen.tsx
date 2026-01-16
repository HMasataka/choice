import React, { useState, useEffect, useRef } from "react";
import { useLocalMedia } from "../hooks";
import {
    MicIcon,
    MicOffIcon,
    VideoIcon,
    VideoOffIcon,
} from "./Icons";

interface JoinScreenProps {
    onJoin: (roomId: string, displayName: string) => void;
    isConnecting: boolean;
    error: string | null;
}

export const JoinScreen: React.FC<JoinScreenProps> = ({
    onJoin,
    isConnecting,
    error,
}) => {
    const [roomId, setRoomId] = useState("");
    const [displayName, setDisplayName] = useState("");
    const videoRef = useRef<HTMLVideoElement>(null);

    const {
        stream,
        getMedia,
        toggleVideo,
        toggleAudio,
        isVideoEnabled,
        isAudioEnabled,
    } = useLocalMedia();

    useEffect(() => {
        getMedia();
    }, [getMedia]);

    useEffect(() => {
        if (videoRef.current && stream) {
            videoRef.current.srcObject = stream;
        }
    }, [stream]);

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault();
        if (roomId.trim() && displayName.trim()) {
            onJoin(roomId.trim(), displayName.trim());
        }
    };

    return (
        <div className="join-screen">
            <div className="join-card">
                <h1>Choice SFU</h1>
                <p>Join a video conference room</p>

                {error && <div className="error-message">{error}</div>}

                <form onSubmit={handleSubmit}>
                    <div className="preview-container">
                        <div className="preview-video">
                            {stream && isVideoEnabled ? (
                                <video ref={videoRef} autoPlay playsInline muted />
                            ) : (
                                <div
                                    className="video-placeholder"
                                    style={{
                                        display: "flex",
                                        alignItems: "center",
                                        justifyContent: "center",
                                        height: "100%",
                                        backgroundColor: "#1e293b",
                                        color: "#94a3b8",
                                    }}
                                >
                                    Camera Off
                                </div>
                            )}
                        </div>
                        <div className="preview-controls">
                            <button
                                type="button"
                                className={`btn btn-icon ${!isAudioEnabled ? "active" : ""}`}
                                onClick={toggleAudio}
                            >
                                {isAudioEnabled ? <MicIcon /> : <MicOffIcon />}
                            </button>
                            <button
                                type="button"
                                className={`btn btn-icon ${!isVideoEnabled ? "active" : ""}`}
                                onClick={toggleVideo}
                            >
                                {isVideoEnabled ? <VideoIcon /> : <VideoOffIcon />}
                            </button>
                        </div>
                    </div>

                    <div className="form-group">
                        <label htmlFor="roomId">Room ID</label>
                        <input
                            id="roomId"
                            type="text"
                            value={roomId}
                            onChange={(e) => setRoomId(e.target.value)}
                            placeholder="Enter room ID"
                            required
                        />
                    </div>

                    <div className="form-group">
                        <label htmlFor="displayName">Display Name</label>
                        <input
                            id="displayName"
                            type="text"
                            value={displayName}
                            onChange={(e) => setDisplayName(e.target.value)}
                            placeholder="Enter your name"
                            required
                        />
                    </div>

                    <button
                        type="submit"
                        className="btn btn-primary"
                        style={{ width: "100%" }}
                        disabled={isConnecting || !roomId.trim() || !displayName.trim()}
                    >
                        {isConnecting ? "Joining..." : "Join Room"}
                    </button>
                </form>
            </div>
        </div>
    );
};
