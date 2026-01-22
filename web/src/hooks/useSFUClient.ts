import { useState, useEffect, useCallback, useRef } from "react";
import { SFUClient, ConnectionState } from "../sfu";

interface UseSFUClientOptions {
    url: string;
    autoConnect?: boolean;
}

interface UseSFUClientReturn {
    client: SFUClient | null;
    connectionState: ConnectionState;
    connect: () => Promise<void>;
    disconnect: () => void;
}

export function useSFUClient(options: UseSFUClientOptions): UseSFUClientReturn {
    const clientRef = useRef<SFUClient | null>(null);
    const [connectionState, setConnectionState] =
        useState<ConnectionState>("disconnected");

    useEffect(() => {
        clientRef.current = new SFUClient({ url: options.url });

        clientRef.current.on("connecting", () => setConnectionState("connecting"));
        clientRef.current.on("connected", () => setConnectionState("connected"));
        clientRef.current.on("disconnected", () =>
            setConnectionState("disconnected"),
        );
        clientRef.current.on("reconnecting", () =>
            setConnectionState("reconnecting"),
        );
        clientRef.current.on("reconnected", () => setConnectionState("connected"));

        if (options.autoConnect) {
            clientRef.current.connect().catch(console.error);
        }

        return () => {
            clientRef.current?.disconnect();
        };
    }, [options.url, options.autoConnect]);

    const connect = useCallback(async () => {
        if (clientRef.current) {
            await clientRef.current.connect();
        }
    }, []);

    const disconnect = useCallback(() => {
        clientRef.current?.disconnect();
    }, []);

    return {
        client: clientRef.current,
        connectionState,
        connect,
        disconnect,
    };
}
