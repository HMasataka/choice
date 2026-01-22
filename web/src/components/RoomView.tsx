import React, { useEffect, useRef, useCallback, useState } from "react";
import { useLocalMedia, useScreenShare } from "@sfu/react-sdk";
import type { Room, LocalParticipant, RemoteParticipant, ConnectionState, SimulcastLayer } from "@sfu/react-sdk";
import { VideoTile } from "./VideoTile";
import {
    MicIcon,
    MicOffIcon,
    VideoIcon,
    VideoOffIcon,
    ScreenShareIcon,
    PhoneOffIcon,
} from "./Icons";

interface RoomViewProps {
    room: Room;
    localParticipant: LocalParticipant;
    participants: RemoteParticipant[];
    displayName: string;
    connectionState: ConnectionState;
    onLeave: () => void;
}

export const RoomView: React.FC<RoomViewProps> = ({
    room,
    localParticipant,
    participants,
    displayName,
    connectionState,
    onLeave,
}) => {
    const [preferredLayers, setPreferredLayers] = useState<Map<string, SimulcastLayer>>(new Map());
    const publishedTrackIds = useRef<Set<string>>(new Set());
    const isPublishing = useRef(false);

    const {
        stream: localStream,
        videoTrack,
        audioTrack,
        toggleVideo,
        toggleAudio,
        videoEnabled,
        audioEnabled,
    } = useLocalMedia({ autoStart: true });

    const {
        isSharing: isScreenSharing,
        startScreenShare,
        stopScreenShare,
    } = useScreenShare(localParticipant);

    // Publish local tracks when available
    useEffect(() => {
        const publishTracks = async () => {
            if (!localStream || isPublishing.current) return;

            isPublishing.current = true;

            try {
                // Publish video track
                if (videoTrack && !publishedTrackIds.current.has("local-video")) {
                    await localParticipant.publish(videoTrack, {
                        simulcast: true,
                        name: "camera",
                        metadata: { source: "camera" },
                    });
                    publishedTrackIds.current.add("local-video");
                    console.log("Published video track");
                }

                // Publish audio track
                if (audioTrack && !publishedTrackIds.current.has("local-audio")) {
                    await localParticipant.publish(audioTrack, {
                        name: "microphone",
                        metadata: { source: "microphone" },
                    });
                    publishedTrackIds.current.add("local-audio");
                    console.log("Published audio track");
                }
            } catch (err) {
                console.error("Failed to publish tracks:", err);
            } finally {
                isPublishing.current = false;
            }
        };

        publishTracks();
    }, [localStream, videoTrack, audioTrack, localParticipant]);

    // Auto-subscribe to remote tracks
    useEffect(() => {
        const subscribeToTracks = async () => {
            for (const participant of participants) {
                for (const track of participant.tracks) {
                    if (!track.isSubscribed) {
                        try {
                            await participant.subscribe(track.id);
                            console.log(`Subscribed to track ${track.id} from ${participant.id}`);
                        } catch (err) {
                            console.error(`Failed to subscribe to track ${track.id}:`, err);
                        }
                    }
                }
            }
        };

        subscribeToTracks();
    }, [participants]);

    const handleLayerChange = useCallback(
        async (trackId: string, layer: SimulcastLayer) => {
            // Find the track and set preferred layer
            for (const participant of participants) {
                const track = participant.getTrack(trackId);
                if (track) {
                    try {
                        await track.setPreferredLayer(layer);
                        setPreferredLayers((prev) => {
                            const next = new Map(prev);
                            next.set(trackId, layer);
                            return next;
                        });
                    } catch (err) {
                        console.error("Failed to set preferred layer:", err);
                    }
                    break;
                }
            }
        },
        [participants],
    );

    const handleLeave = useCallback(() => {
        // Just call onLeave which handles room.leave via useRoom hook
        onLeave();
    }, [onLeave]);

    // Collect remote video tracks for rendering
    const remoteVideoTracks: Array<{
        participantId: string;
        trackId: string;
        stream: MediaStream | null;
        displayName: string;
    }> = [];

    for (const participant of participants) {
        for (const track of participant.tracks) {
            if (track.kind === "video" && track.isSubscribed && track.mediaStreamTrack) {
                remoteVideoTracks.push({
                    participantId: participant.id,
                    trackId: track.id,
                    stream: new MediaStream([track.mediaStreamTrack]),
                    displayName: (participant.metadata?.displayName as string) || participant.id,
                });
            }
        }
    }

    return (
        <>
            <header className="header">
                <h1>Room: {room.id}</h1>
                <div className={`connection-status ${connectionState}`}>
                    {connectionState === "connected" ? "Connected" : connectionState}
                </div>
            </header>

            <main className="main">
                <div className="video-grid">
                    <VideoTile
                        stream={localStream}
                        label={`${displayName} (You)`}
                        isLocal
                        muted
                    />

                    {remoteVideoTracks.map(({ trackId, stream, displayName: name }) => (
                        <VideoTile
                            key={trackId}
                            stream={stream}
                            label={name}
                            showLayerSelector
                            currentLayer={preferredLayers.get(trackId) ?? "h"}
                            onLayerChange={(layer) => handleLayerChange(trackId, layer)}
                        />
                    ))}
                </div>

                <aside className="sidebar">
                    <section className="sidebar-section">
                        <h2>Participants ({participants.length + 1})</h2>
                        <div className="participant-list">
                            <div className="participant-item self">
                                <div className="participant-avatar">
                                    {displayName.charAt(0).toUpperCase()}
                                </div>
                                <span className="participant-name">{displayName}</span>
                            </div>
                            {participants.map((p) => {
                                const name = (p.metadata?.displayName as string) || p.id || "Unknown";
                                return (
                                    <div key={p.id} className="participant-item">
                                        <div className="participant-avatar">
                                            {name.charAt(0).toUpperCase()}
                                        </div>
                                        <span className="participant-name">{name}</span>
                                    </div>
                                );
                            })}
                        </div>
                    </section>
                </aside>
            </main>

            <div className="controls">
                <button
                    className={`btn btn-icon ${!audioEnabled ? "active" : ""}`}
                    onClick={toggleAudio}
                    title={audioEnabled ? "Mute" : "Unmute"}
                >
                    {audioEnabled ? <MicIcon /> : <MicOffIcon />}
                </button>

                <button
                    className={`btn btn-icon ${!videoEnabled ? "active" : ""}`}
                    onClick={toggleVideo}
                    title={videoEnabled ? "Turn off camera" : "Turn on camera"}
                >
                    {videoEnabled ? <VideoIcon /> : <VideoOffIcon />}
                </button>

                <button
                    className={`btn btn-icon ${isScreenSharing ? "active" : ""}`}
                    onClick={() => isScreenSharing ? stopScreenShare() : startScreenShare()}
                    title={isScreenSharing ? "Stop sharing" : "Share screen"}
                >
                    <ScreenShareIcon />
                </button>

                <button className="btn btn-danger" onClick={handleLeave} title="Leave">
                    <PhoneOffIcon />
                    <span>Leave</span>
                </button>
            </div>
        </>
    );
};
