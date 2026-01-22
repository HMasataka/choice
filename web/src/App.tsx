import React, { useState, useCallback, useRef } from "react";
import { SFUClient, ConnectionState, ParticipantInfo } from "./sfu";
import { JoinScreen, Room } from "./components";

type AppState = "join" | "connecting" | "room";

const WS_URL = `${window.location.protocol === "https:" ? "wss:" : "ws:"}//${window.location.host}/ws`;

export const App: React.FC = () => {
    const [appState, setAppState] = useState<AppState>("join");
    const [error, setError] = useState<string | null>(null);
    const [displayName, setDisplayName] = useState("");
    const [connectionState, setConnectionState] =
        useState<ConnectionState>("disconnected");
    const [initialParticipants, setInitialParticipants] = useState<ParticipantInfo[]>([]);
    const clientRef = useRef<SFUClient | null>(null);

    const handleJoin = useCallback(async (roomId: string, name: string) => {
        setError(null);
        setDisplayName(name);
        setAppState("connecting");

        try {
            const client = new SFUClient({ url: WS_URL });
            clientRef.current = client;

            client.on("connecting", () => setConnectionState("connecting"));
            client.on("connected", () => setConnectionState("connected"));
            client.on("disconnected", () => setConnectionState("disconnected"));
            client.on("reconnecting", () => setConnectionState("reconnecting"));
            client.on("reconnected", () => setConnectionState("connected"));
            client.on("error", (err) => {
                console.error("SFU error:", err);
                if (err.fatal) {
                    setError(err.message);
                    setAppState("join");
                }
            });

            await client.connect();

            // TODO: In production, token should come from auth server
            // For now, create a mock token with room info
            const mockToken = btoa(
                JSON.stringify({
                    roomId,
                    userId: `user-${Date.now()}`,
                    displayName: name,
                }),
            );

            const joinResult = await client.join(mockToken, { displayName: name });
            console.log("Join result participants:", joinResult.participants);
            setInitialParticipants(joinResult.participants ?? []);
            setAppState("room");
        } catch (err) {
            console.error("Failed to join:", err);
            setError((err as Error).message);
            setAppState("join");
            clientRef.current?.disconnect();
            clientRef.current = null;
        }
    }, []);

    const handleLeave = useCallback(() => {
        clientRef.current?.disconnect();
        clientRef.current = null;
        setAppState("join");
        setConnectionState("disconnected");
    }, []);

    return (
        <div className="app">
            {appState === "join" && (
                <JoinScreen
                    onJoin={handleJoin}
                    isConnecting={false}
                    error={error}
                />
            )}

            {appState === "connecting" && (
                <JoinScreen
                    onJoin={handleJoin}
                    isConnecting={true}
                    error={error}
                />
            )}

            {appState === "room" && clientRef.current && (
                <Room
                    client={clientRef.current}
                    displayName={displayName}
                    initialParticipants={initialParticipants}
                    onLeave={handleLeave}
                />
            )}

            {connectionState === "reconnecting" && (
                <div
                    style={{
                        position: "fixed",
                        top: 0,
                        left: 0,
                        right: 0,
                        padding: "8px",
                        backgroundColor: "#f59e0b",
                        color: "black",
                        textAlign: "center",
                        zIndex: 1000,
                    }}
                >
                    Reconnecting...
                </div>
            )}
        </div>
    );
};
