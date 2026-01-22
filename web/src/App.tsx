import React, { useState, useCallback, useEffect } from "react";
import { useSFUClient, useRoom } from "@sfu/react-sdk";
import { JoinScreen, RoomView } from "./components";

type AppState = "join" | "connecting" | "room";

const WS_URL = `${window.location.protocol === "https:" ? "wss:" : "ws:"}//${window.location.host}/ws`;

export const App: React.FC = () => {
    const [appState, setAppState] = useState<AppState>("join");
    const [error, setError] = useState<string | null>(null);
    const [displayName, setDisplayName] = useState("");
    const [token, setToken] = useState("");

    const { client, connectionState, connect, error: clientError } = useSFUClient({
        url: WS_URL,
    });

    const { room, participants, localParticipant, leave, error: roomError } = useRoom({
        client,
        token,
        autoJoin: true, // Auto-join when token is set and client is connected
        joinOptions: {
            metadata: { displayName },
        },
    });

    // Transition to room state when room is successfully joined
    useEffect(() => {
        if (room !== null && localParticipant !== null && appState === "connecting") {
            setAppState("room");
        }
    }, [room, localParticipant, appState]);

    // Handle room error
    useEffect(() => {
        if (roomError !== null && appState === "connecting") {
            setError(roomError.message);
            setAppState("join");
        }
    }, [roomError, appState]);

    const handleJoin = useCallback(async (roomId: string, name: string) => {
        setError(null);
        setDisplayName(name);
        setAppState("connecting");

        try {
            // Wait for client to be initialized before connecting
            if (client === null) {
                throw new Error("Client not yet initialized");
            }

            // Connect to signaling server first
            await connect();

            // Create mock token with room info
            // TODO: In production, token should come from auth server
            const mockToken = btoa(
                JSON.stringify({
                    roomId,
                    userId: `user-${Date.now()}`,
                    displayName: name,
                }),
            );

            // Setting the token will trigger autoJoin via useRoom hook
            setToken(mockToken);
        } catch (err) {
            console.error("Failed to connect:", err);
            setError((err as Error).message);
            setAppState("join");
        }
    }, [client, connect]);

    const handleLeave = useCallback(async () => {
        try {
            await leave();
        } catch (err) {
            console.error("Failed to leave:", err);
        }
        setAppState("join");
        setToken("");
    }, [leave]);

    // Combine errors
    const displayError = error || (clientError?.message ?? null) || (roomError?.message ?? null);

    return (
        <div className="app">
            {appState === "join" && (
                <JoinScreen
                    onJoin={handleJoin}
                    isConnecting={false}
                    error={displayError}
                />
            )}

            {appState === "connecting" && (
                <JoinScreen
                    onJoin={handleJoin}
                    isConnecting={true}
                    error={displayError}
                />
            )}

            {appState === "room" && room && localParticipant && (
                <RoomView
                    room={room}
                    localParticipant={localParticipant}
                    participants={participants}
                    displayName={displayName}
                    connectionState={connectionState}
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
