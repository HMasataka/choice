import React, { useEffect, useRef, useCallback, useState } from "react";
import { SFUClient, ParticipantInfo, TrackInfo, SimulcastLayer } from "../sfu";
import { useLocalMedia, useScreenShare } from "../hooks";
import { VideoTile } from "./VideoTile";
import {
    MicIcon,
    MicOffIcon,
    VideoIcon,
    VideoOffIcon,
    ScreenShareIcon,
    PhoneOffIcon,
} from "./Icons";

interface RemoteParticipant {
    id: string;
    metadata: Record<string, unknown>;
    tracks: Map<string, { info: TrackInfo; stream: MediaStream | null }>;
}

interface RoomProps {
    client: SFUClient;
    displayName: string;
    initialParticipants?: ParticipantInfo[];
    onLeave: () => void;
}

export const Room: React.FC<RoomProps> = ({ client, displayName, initialParticipants, onLeave }) => {
    const [participants, setParticipants] = useState<Map<string, RemoteParticipant>>(() => {
        // Initialize with existing participants from join result
        const initial = new Map<string, RemoteParticipant>();
        if (initialParticipants) {
            for (const p of initialParticipants) {
                initial.set(p.id, {
                    id: p.id,
                    metadata: p.metadata ?? {},
                    tracks: new Map(
                        (p.tracks ?? []).map((t) => [t.trackId, { info: t, stream: null }]),
                    ),
                });
            }
        }
        return initial;
    });
    const [preferredLayers, setPreferredLayers] = useState<
        Map<string, SimulcastLayer>
    >(new Map());
    const publishedTrackIds = useRef<Set<string>>(new Set());
    const isPublishing = useRef(false);

    const {
        stream: localStream,
        getMedia,
        toggleVideo,
        toggleAudio,
        isVideoEnabled,
        isAudioEnabled,
    } = useLocalMedia();

    const {
        stream: screenStream,
        isSharing: isScreenSharing,
        startScreenShare,
        stopScreenShare,
    } = useScreenShare();

    const screenTrackIdRef = useRef<string | null>(null);

    useEffect(() => {
        getMedia();
    }, [getMedia]);

    // Debug: Log whenever participants state changes
    useEffect(() => {
        console.log("=== PARTICIPANTS STATE CHANGED ===");
        participants.forEach((p, id) => {
            console.log(`  Participant ${id}:`);
            p.tracks.forEach((t, tid) => {
                console.log(`    Track ${tid}: kind=${t.info.kind}, hasStream=${!!t.stream}, streamActive=${t.stream?.active}`);
            });
        });
        console.log("=================================");
    }, [participants]);

    // Track whether local tracks have been published (peer connection established)
    const [localTracksPublished, setLocalTracksPublished] = useState(false);

    // Subscribe to existing participants' tracks after local tracks are published
    // (peer connection must exist on server before we can subscribe)
    useEffect(() => {
        if (!localTracksPublished) return;
        if (!initialParticipants || initialParticipants.length === 0) return;

        const subscribeToExistingTracks = async () => {
            console.log("Local tracks published, now subscribing to existing tracks");
            for (const participant of initialParticipants) {
                if (!participant.tracks) continue;

                for (const track of participant.tracks) {
                    console.log(`Subscribing to existing track: ${track.trackId} from ${participant.id}`);
                    try {
                        const subscriptionId = await client.subscribe(participant.id, track.trackId, "h");
                        console.log(`Subscribed to existing track: ${track.trackId}`, subscriptionId);
                    } catch (err) {
                        console.error(`Failed to subscribe to existing track ${track.trackId}:`, err);
                    }
                }
            }
        };

        subscribeToExistingTracks();
    }, [localTracksPublished, initialParticipants, client]);

    useEffect(() => {
        const publishTracks = async () => {
            if (!localStream) return;

            // Prevent concurrent publishing (race condition guard)
            if (isPublishing.current) return;

            const videoTrack = localStream.getVideoTracks()[0];
            const audioTrack = localStream.getAudioTracks()[0];

            // Collect tracks to publish in batch
            const tracksToPublish: Array<{
                track: MediaStreamTrack;
                options: { simulcast?: boolean; name?: string; metadata?: Record<string, unknown> };
                localKey: string;
            }> = [];

            if (videoTrack && !publishedTrackIds.current.has("local-video")) {
                tracksToPublish.push({
                    track: videoTrack,
                    options: {
                        simulcast: true,
                        name: "camera",
                        metadata: { source: "camera" },
                    },
                    localKey: "local-video",
                });
            }

            if (audioTrack && !publishedTrackIds.current.has("local-audio")) {
                tracksToPublish.push({
                    track: audioTrack,
                    options: {
                        name: "microphone",
                        metadata: { source: "microphone" },
                    },
                    localKey: "local-audio",
                });
            }

            if (tracksToPublish.length === 0) return;

            // Mark as publishing BEFORE async call to prevent race condition
            isPublishing.current = true;

            try {
                // Publish all tracks with a single renegotiation
                console.log(`Publishing ${tracksToPublish.length} tracks in batch`);
                const trackIds = await client.publishAll(
                    tracksToPublish.map(({ track, options }) => ({ track, options }))
                );

                // Mark all as published
                tracksToPublish.forEach(({ localKey }, index) => {
                    publishedTrackIds.current.add(localKey);
                    console.log(`Published ${localKey} track:`, trackIds[index]);
                });

                // Signal that local tracks have been published (peer connection established)
                setLocalTracksPublished(true);
            } catch (err) {
                console.error("Failed to publish tracks:", err);
            } finally {
                isPublishing.current = false;
            }
        };

        publishTracks();
    }, [localStream, client]);

    useEffect(() => {
        const publishScreenTrack = async () => {
            if (!screenStream) {
                if (screenTrackIdRef.current) {
                    try {
                        await client.unpublish(screenTrackIdRef.current);
                    } catch (err) {
                        console.error("Failed to unpublish screen track:", err);
                    }
                    screenTrackIdRef.current = null;
                }
                return;
            }

            const videoTrack = screenStream.getVideoTracks()[0];
            if (videoTrack && !screenTrackIdRef.current) {
                try {
                    const trackId = await client.publish(videoTrack, {
                        simulcast: true,
                        name: "screen",
                        metadata: { source: "screen" },
                    });
                    screenTrackIdRef.current = trackId;
                    console.log("Published screen track:", trackId);
                } catch (err) {
                    console.error("Failed to publish screen track:", err);
                }
            }
        };

        publishScreenTrack();
    }, [screenStream, client]);

    useEffect(() => {
        const handleParticipantJoined = (participant: ParticipantInfo) => {
            console.log("participantJoined event:", participant);
            setParticipants((prev) => {
                const next = new Map(prev);
                const tracks = participant.tracks ?? [];
                next.set(participant.id, {
                    id: participant.id,
                    metadata: participant.metadata ?? {},
                    tracks: new Map(
                        tracks.map((t) => [t.trackId, { info: t, stream: null }]),
                    ),
                });
                return next;
            });
        };

        const handleParticipantLeft = (data: { id: string }) => {
            setParticipants((prev) => {
                const next = new Map(prev);
                next.delete(data.id);
                return next;
            });
        };

        const handleTrackPublished = (data: {
            publisherId: string;
            track: TrackInfo;
        }) => {
            console.log("trackPublished event:", data);

            // Ignore our own tracks (shouldn't happen after server fix, but just in case)
            if (data.publisherId === client.participantId) {
                console.log("Ignoring own track:", data.track.trackId);
                return;
            }

            setParticipants((prev) => {
                const next = new Map(prev);
                const existing = next.get(data.publisherId);

                // Create new participant object (or create if not exists)
                // Must create new objects at each level to trigger React re-render
                const newTracks = existing ? new Map(existing.tracks) : new Map();
                newTracks.set(data.track.trackId, {
                    info: data.track,
                    stream: null,
                });

                const newParticipant = existing
                    ? { ...existing, tracks: newTracks }
                    : {
                        id: data.publisherId,
                        metadata: {},
                        tracks: newTracks,
                    };

                if (!existing) {
                    console.log("Creating participant for trackPublished:", data.publisherId);
                }

                next.set(data.publisherId, newParticipant);
                return next;
            });

            client
                .subscribe(data.publisherId, data.track.trackId, "h")
                .then((subscriptionId) => {
                    console.log("Subscribed to track:", data.track.trackId, subscriptionId);
                })
                .catch(console.error);
        };

        const handleTrackUnpublished = (data: {
            publisherId: string;
            trackId: string;
        }) => {
            setParticipants((prev) => {
                const next = new Map(prev);
                const participant = next.get(data.publisherId);
                if (participant) {
                    // Create new objects to trigger React re-render
                    const newTracks = new Map(participant.tracks);
                    newTracks.delete(data.trackId);
                    next.set(data.publisherId, { ...participant, tracks: newTracks });
                }
                return next;
            });
        };

        const handleTrackReceived = (data: {
            track: MediaStreamTrack;
            streams: readonly MediaStream[];
        }) => {
            // Use provided stream or create a new one if none provided
            let stream = data.streams[0];
            if (!stream) {
                console.log("handleTrackReceived: No stream provided, creating new MediaStream");
                stream = new MediaStream([data.track]);
            }

            console.log("handleTrackReceived called:", {
                trackKind: data.track.kind,
                trackId: data.track.id,
                streamId: stream.id,
                streamActive: stream.active,
                trackEnabled: data.track.enabled,
                trackReadyState: data.track.readyState,
            });

            setParticipants((prev) => {
                const next = new Map(prev);

                // Debug: log current state
                console.log("handleTrackReceived state check:", {
                    participantCount: next.size,
                    participants: Array.from(next.entries()).map(([id, p]) => ({
                        id,
                        trackCount: p.tracks.size,
                        tracks: Array.from(p.tracks.entries()).map(([tid, td]) => ({
                            trackId: tid,
                            kind: td.info.kind,
                            hasStream: !!td.stream,
                        })),
                    })),
                });

                for (const [participantId, participant] of next) {
                    for (const [trackId, trackData] of participant.tracks) {
                        if (trackData.info.kind === data.track.kind && !trackData.stream) {
                            // Create new objects to trigger React re-render
                            // (mutating existing objects doesn't trigger re-render)
                            const newTracks = new Map(participant.tracks);
                            newTracks.set(trackId, { ...trackData, stream });
                            next.set(participantId, { ...participant, tracks: newTracks });
                            console.log("Track received and matched:", trackId, "kind:", trackData.info.kind);
                            return next;
                        }
                    }
                }
                console.log("WARNING: No matching track found for kind:", data.track.kind);
                return next;
            });
        };

        client.on("participantJoined", handleParticipantJoined);
        client.on("participantLeft", handleParticipantLeft);
        client.on("trackPublished", handleTrackPublished);
        client.on("trackUnpublished", handleTrackUnpublished);
        client.on("trackReceived", handleTrackReceived);

        return () => {
            client.off("participantJoined", handleParticipantJoined);
            client.off("participantLeft", handleParticipantLeft);
            client.off("trackPublished", handleTrackPublished);
            client.off("trackUnpublished", handleTrackUnpublished);
            client.off("trackReceived", handleTrackReceived);
        };
    }, [client]);

    const handleLayerChange = useCallback(
        (trackId: string, layer: SimulcastLayer) => {
            client.setPreferredLayer(trackId, layer).catch(console.error);
            setPreferredLayers((prev) => {
                const next = new Map(prev);
                next.set(trackId, layer);
                return next;
            });
        },
        [client],
    );

    const handleLeave = useCallback(async () => {
        try {
            await client.leave();
        } catch (err) {
            console.error("Failed to leave:", err);
        }
        onLeave();
    }, [client, onLeave]);

    const remoteVideoTracks: Array<{
        participantId: string;
        trackId: string;
        stream: MediaStream | null;
        displayName: string;
    }> = [];

    participants.forEach((participant) => {
        participant.tracks.forEach((trackData, trackId) => {
            if (trackData.info.kind === "video") {
                remoteVideoTracks.push({
                    participantId: participant.id,
                    trackId,
                    stream: trackData.stream,
                    displayName: (participant.metadata.displayName as string) || participant.id,
                });
            }
        });
    });

    // Debug: log what we're rendering
    if (remoteVideoTracks.length > 0) {
        remoteVideoTracks.forEach(t => {
            console.log(`Rendering video track: trackId=${t.trackId}, hasStream=${!!t.stream}, streamActive=${t.stream?.active}`);
        });
    }

    return (
        <>
            <header className="header">
                <h1>Room: {client.roomId}</h1>
                <div className="connection-status connected">Connected</div>
            </header>

            <main className="main">
                <div className="video-grid">
                    <VideoTile
                        stream={localStream}
                        label={`${displayName} (You)`}
                        isLocal
                        muted
                    />

                    {isScreenSharing && screenStream && (
                        <VideoTile stream={screenStream} label="Screen Share" />
                    )}

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
                        <h2>Participants ({participants.size + 1})</h2>
                        <div className="participant-list">
                            <div className="participant-item self">
                                <div className="participant-avatar">
                                    {displayName.charAt(0).toUpperCase()}
                                </div>
                                <span className="participant-name">{displayName}</span>
                            </div>
                            {Array.from(participants.values()).map((p) => {
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
                    className={`btn btn-icon ${!isAudioEnabled ? "active" : ""}`}
                    onClick={toggleAudio}
                    title={isAudioEnabled ? "Mute" : "Unmute"}
                >
                    {isAudioEnabled ? <MicIcon /> : <MicOffIcon />}
                </button>

                <button
                    className={`btn btn-icon ${!isVideoEnabled ? "active" : ""}`}
                    onClick={toggleVideo}
                    title={isVideoEnabled ? "Turn off camera" : "Turn on camera"}
                >
                    {isVideoEnabled ? <VideoIcon /> : <VideoOffIcon />}
                </button>

                <button
                    className={`btn btn-icon ${isScreenSharing ? "active" : ""}`}
                    onClick={isScreenSharing ? stopScreenShare : startScreenShare}
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
