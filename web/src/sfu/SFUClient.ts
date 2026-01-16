import {
    ConnectionState,
    JoinResult,
    LayerChangedEvent,
    ParticipantInfo,
    PublishResult,
    ServerError,
    SimulcastLayer,
    SubscribeResult,
    TrackInfo,
} from "./types";

type EventHandler<T = void> = T extends void ? () => void : (data: T) => void;

interface SFUClientEvents {
    connecting: EventHandler;
    connected: EventHandler;
    disconnected: EventHandler<string>;
    reconnecting: EventHandler;
    reconnected: EventHandler;
    participantJoined: EventHandler<ParticipantInfo>;
    participantLeft: EventHandler<{ id: string; reason: string }>;
    trackPublished: EventHandler<{
        publisherId: string;
        track: TrackInfo;
    }>;
    trackUnpublished: EventHandler<{ publisherId: string; trackId: string }>;
    trackReceived: EventHandler<{
        track: MediaStreamTrack;
        streams: readonly MediaStream[];
    }>;
    layerChanged: EventHandler<LayerChangedEvent>;
    error: EventHandler<ServerError>;
}

interface SFUClientConfig {
    url: string;
    reconnect?: {
        maxAttempts: number;
        initialDelay: number;
        maxDelay: number;
        factor: number;
    };
}

export class SFUClient {
    private config: SFUClientConfig;
    private ws: WebSocket | null = null;
    private pc: RTCPeerConnection | null = null;
    private pendingRequests = new Map<
        string,
        {
            resolve: (value: unknown) => void;
            reject: (error: Error) => void;
        }
    >();
    private sessionId: string | null = null;
    private connectionState: ConnectionState = "disconnected";
    private reconnectAttempts = 0;
    private eventHandlers = new Map<string, Set<EventHandler<unknown>>>();
    private isNegotiating = false;
    private pendingNegotiation = false;
    private pendingCandidates: Array<{
        candidate: string;
        sdpMid: string;
        sdpMLineIndex: number;
    }> = [];

    public participantId: string | null = null;
    public roomId: string | null = null;

    constructor(config: SFUClientConfig) {
        this.config = {
            ...config,
            reconnect: config.reconnect ?? {
                maxAttempts: 5,
                initialDelay: 1000,
                maxDelay: 30000,
                factor: 2,
            },
        };
    }

    on<K extends keyof SFUClientEvents>(
        event: K,
        handler: SFUClientEvents[K],
    ): void {
        if (!this.eventHandlers.has(event)) {
            this.eventHandlers.set(event, new Set());
        }
        this.eventHandlers.get(event)!.add(handler as EventHandler<unknown>);
    }

    off<K extends keyof SFUClientEvents>(
        event: K,
        handler: SFUClientEvents[K],
    ): void {
        this.eventHandlers.get(event)?.delete(handler as EventHandler<unknown>);
    }

    private emit<K extends keyof SFUClientEvents>(
        event: K,
        ...args: Parameters<SFUClientEvents[K]>
    ): void {
        const handlers = this.eventHandlers.get(event);
        if (handlers) {
            handlers.forEach((handler) => {
                (handler as (...args: unknown[]) => void)(...args);
            });
        }
    }

    getConnectionState(): ConnectionState {
        return this.connectionState;
    }

    async connect(): Promise<void> {
        if (this.connectionState !== "disconnected") {
            return;
        }

        this.connectionState = "connecting";
        this.emit("connecting");

        return new Promise((resolve, reject) => {
            this.ws = new WebSocket(this.config.url);

            this.ws.onopen = () => {
                this.connectionState = "connected";
                this.reconnectAttempts = 0;
                this.emit("connected");
                resolve();
            };

            this.ws.onerror = (error) => {
                console.error("WebSocket error:", error);
                reject(new Error("WebSocket connection failed"));
            };

            this.ws.onmessage = (event) => {
                this.handleMessage(JSON.parse(event.data));
            };

            this.ws.onclose = (event) => {
                this.handleClose(event.code, event.reason);
            };
        });
    }

    async join(token: string, metadata?: Record<string, unknown>): Promise<JoinResult> {
        const result = (await this.sendRequest("join", {
            token,
            sessionId: this.sessionId,
            metadata,
        })) as JoinResult;

        console.log("Join result:", result);
        console.log("Existing participants:", result.participants);

        this.sessionId = result.sessionId;
        this.participantId = result.participantId;
        this.roomId = result.roomId;

        this.pc = new RTCPeerConnection({
            iceServers: result.iceServers,
            bundlePolicy: "max-bundle",
            rtcpMuxPolicy: "require",
        });

        this.setupPeerConnection();

        return result;
    }

    private setupPeerConnection(): void {
        if (!this.pc) return;

        this.pc.onicecandidate = (event) => {
            if (event.candidate) {
                // In ICE Lite mode, server may not need client ICE candidates
                // Errors here are expected and can be ignored
                this.sendRequest("candidate", {
                    candidate: event.candidate.candidate,
                    sdpMid: event.candidate.sdpMid,
                    sdpMLineIndex: event.candidate.sdpMLineIndex,
                }).catch(() => {
                    // Silently ignore ICE candidate errors (expected in ICE Lite mode)
                });
            }
        };

        this.pc.ontrack = (event) => {
            console.log("SFUClient ontrack fired:", {
                trackId: event.track.id,
                trackKind: event.track.kind,
                trackEnabled: event.track.enabled,
                trackReadyState: event.track.readyState,
                streamsCount: event.streams.length,
                stream0Id: event.streams[0]?.id,
                stream0Active: event.streams[0]?.active,
            });
            this.emit("trackReceived", {
                track: event.track,
                streams: event.streams,
            });
        };

        this.pc.oniceconnectionstatechange = () => {
            console.log("ICE state:", this.pc?.iceConnectionState);
            if (this.pc?.iceConnectionState === "failed") {
                this.restartICE().catch(console.error);
            }
        };

        this.pc.onconnectionstatechange = () => {
            console.log("Connection state:", this.pc?.connectionState);

            // Debug: Log outbound RTP stats when connected
            if (this.pc?.connectionState === "connected") {
                // Log stats at multiple intervals to check if packets start flowing
                setTimeout(() => this.logOutboundStats(), 1000);
                setTimeout(() => this.logOutboundStats(), 3000);
                setTimeout(() => this.logOutboundStats(), 5000);
            }
        };
    }

    private async logOutboundStats(): Promise<void> {
        if (!this.pc) return;

        // Log ICE and connection details
        console.log("[Debug] Connection state details:");
        console.log(`  ICE connection state: ${this.pc.iceConnectionState}`);
        console.log(`  ICE gathering state: ${this.pc.iceGatheringState}`);
        console.log(`  Connection state: ${this.pc.connectionState}`);
        console.log(`  Signaling state: ${this.pc.signalingState}`);

        // Log transceiver directions
        console.log("[Debug] Transceivers after connection:");
        this.pc.getTransceivers().forEach((t, i) => {
            const senderTrack = t.sender.track;
            console.log(`  transceiver[${i}]: mid=${t.mid}, direction=${t.direction}, currentDirection=${t.currentDirection}, kind=${t.receiver.track?.kind}`);
            console.log(`    sender.track: kind=${senderTrack?.kind}, enabled=${senderTrack?.enabled}, readyState=${senderTrack?.readyState}, muted=${senderTrack?.muted}`);
        });

        // Log senders with track state
        console.log("[Debug] Senders:");
        this.pc.getSenders().forEach((s, i) => {
            const t = s.track;
            const params = s.getParameters();
            const encoding = params.encodings?.[0] as Record<string, unknown> | undefined;
            console.log(`  sender[${i}]: kind=${t?.kind}, enabled=${t?.enabled}, readyState=${t?.readyState}, muted=${t?.muted}, active=${encoding?.active}`);
        });

        const stats = await this.pc.getStats();
        stats.forEach((report) => {
            if (report.type === "outbound-rtp") {
                const r = report as RTCOutboundRtpStreamStats;
                console.log(`[Stats] ${r.kind}: bytesSent=${r.bytesSent}, packetsSent=${r.packetsSent}, ssrc=${r.ssrc}`);
            }
            // Also check candidate-pair for connectivity
            if (report.type === "candidate-pair") {
                const cp = report as RTCIceCandidatePairStats;
                if (cp.state === "succeeded" || cp.nominated) {
                    console.log(`[Stats] candidate-pair: state=${cp.state}, nominated=${cp.nominated}, bytesSent=${cp.bytesSent}, bytesReceived=${cp.bytesReceived}`);
                }
            }
        });
    }

    async publish(
        track: MediaStreamTrack,
        options: {
            simulcast?: boolean;
            name?: string;
            metadata?: Record<string, unknown>;
            skipNegotiation?: boolean; // For batch publishing
        } = {},
    ): Promise<string> {
        if (!this.pc) {
            throw new Error("Not connected");
        }

        const useSimulcast = options.simulcast ?? (track.kind === "video");

        if (track.kind === "video" && useSimulcast) {
            // Use addTransceiver for simulcast to set encodings upfront
            this.pc.addTransceiver(track, {
                direction: "sendonly",
                sendEncodings: [
                    { rid: "h", maxBitrate: 2500000 },
                    { rid: "m", maxBitrate: 500000, scaleResolutionDownBy: 2 },
                    { rid: "l", maxBitrate: 150000, scaleResolutionDownBy: 4 },
                ],
            });
        } else {
            // Use addTrack for non-simulcast tracks
            console.log(`Adding ${track.kind} track to PeerConnection, id=${track.id}, enabled=${track.enabled}`);
            this.pc.addTrack(track);
        }

        // Debug: Log all senders after adding track
        console.log("Current senders after addTrack:");
        this.pc.getSenders().forEach((sender, i) => {
            const t = sender.track;
            console.log(`  sender[${i}]: kind=${t?.kind}, id=${t?.id}, enabled=${t?.enabled}`);
        });

        const result = (await this.sendRequest("publish", {
            kind: track.kind,
            simulcast: useSimulcast,
            name: options.name,
            metadata: options.metadata,
        })) as PublishResult;

        // Skip renegotiation if batch publishing (will be called separately)
        if (!options.skipNegotiation) {
            await this.renegotiate();
        }

        return result.trackId;
    }

    /**
     * Publish multiple tracks at once with a single renegotiation.
     * This prevents race conditions that occur when publishing tracks sequentially.
     */
    async publishAll(
        tracks: Array<{
            track: MediaStreamTrack;
            options?: {
                simulcast?: boolean;
                name?: string;
                metadata?: Record<string, unknown>;
            };
        }>,
    ): Promise<string[]> {
        if (!this.pc) {
            throw new Error("Not connected");
        }

        console.log(`Publishing ${tracks.length} tracks in batch`);

        const trackIds: string[] = [];

        // Add all tracks and send publish requests, but skip individual renegotiations
        for (const { track, options = {} } of tracks) {
            const trackId = await this.publish(track, {
                ...options,
                skipNegotiation: true, // Don't renegotiate for each track
            });
            trackIds.push(trackId);
        }

        // Single renegotiation after all tracks are added
        console.log("All tracks added, performing single renegotiation");
        await this.renegotiate();

        return trackIds;
    }

    /**
     * Force a renegotiation. Useful when using skipNegotiation option.
     */
    async forceRenegotiate(): Promise<void> {
        await this.renegotiate();
    }

    async unpublish(trackId: string): Promise<void> {
        await this.sendRequest("unpublish", { trackId });
        await this.renegotiate();
    }

    async subscribe(
        publisherId: string,
        trackId: string,
        preferredLayer: SimulcastLayer = "h",
    ): Promise<string> {
        const result = (await this.sendRequest("subscribe", {
            publisherId,
            trackId,
            preferredLayer,
        })) as SubscribeResult;

        return result.subscriptionId;
    }

    async unsubscribe(subscriptionId: string): Promise<void> {
        await this.sendRequest("unsubscribe", { subscriptionId });
    }

    async setPreferredLayer(
        trackId: string,
        layer: SimulcastLayer,
    ): Promise<void> {
        await this.sendRequest("setPreferredLayer", { trackId, layer });
    }

    async leave(): Promise<void> {
        await this.sendRequest("leave", {});
        this.sessionId = null;
        this.participantId = null;
        this.roomId = null;
    }

    disconnect(): void {
        this.pc?.close();
        this.pc = null;
        this.ws?.close();
        this.ws = null;
        this.connectionState = "disconnected";
        this.sessionId = null;
        this.participantId = null;
        this.roomId = null;
    }

    private async renegotiate(): Promise<void> {
        if (!this.pc) return;

        // Prevent concurrent negotiations
        if (this.isNegotiating) {
            this.pendingNegotiation = true;
            return;
        }

        this.isNegotiating = true;
        try {
            const offer = await this.pc.createOffer();
            await this.pc.setLocalDescription(offer);

            // Debug: Log m= lines in offer SDP
            const offerSdp = this.pc.localDescription?.sdp || "";
            console.log("Sending offer to server, m-lines:");
            offerSdp.split("\n").forEach((line) => {
                if (line.startsWith("m=") || line.startsWith("a=sendonly") || line.startsWith("a=sendrecv") || line.startsWith("a=recvonly")) {
                    console.log("  " + line.trim());
                }
            });

            const result = (await this.sendRequest("offer", {
                sdp: this.pc.localDescription?.sdp,
            })) as { sdp: string };

            await this.pc.setRemoteDescription({
                type: "answer",
                sdp: result.sdp,
            });

            // Process any queued ICE candidates
            await this.processPendingCandidates();
        } finally {
            this.isNegotiating = false;
            // If another negotiation was requested while we were negotiating, do it now
            if (this.pendingNegotiation) {
                this.pendingNegotiation = false;
                await this.renegotiate();
            }
        }
    }

    private async restartICE(): Promise<void> {
        if (!this.pc) return;

        const offer = await this.pc.createOffer({ iceRestart: true });
        await this.pc.setLocalDescription(offer);

        const result = (await this.sendRequest("offer", {
            sdp: this.pc.localDescription?.sdp,
        })) as { sdp: string };

        await this.pc.setRemoteDescription({
            type: "answer",
            sdp: result.sdp,
        });

        // Process any queued ICE candidates
        await this.processPendingCandidates();
    }

    private sendRequest(
        method: string,
        params: Record<string, unknown>,
    ): Promise<unknown> {
        return new Promise((resolve, reject) => {
            if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
                reject(new Error("WebSocket not connected"));
                return;
            }

            const id = crypto.randomUUID();
            this.pendingRequests.set(id, { resolve, reject });

            this.ws.send(
                JSON.stringify({
                    jsonrpc: "2.0",
                    id,
                    method,
                    params,
                }),
            );

            setTimeout(() => {
                if (this.pendingRequests.has(id)) {
                    this.pendingRequests.delete(id);
                    reject(new Error("Request timeout"));
                }
            }, 10000);
        });
    }

    private handleMessage(message: {
        id?: string;
        method?: string;
        params?: unknown;
        result?: unknown;
        error?: { code: number; message: string; data?: { details?: string } };
    }): void {
        if (message.id !== undefined) {
            const pending = this.pendingRequests.get(message.id);
            if (pending) {
                this.pendingRequests.delete(message.id);
                if (message.error) {
                    const details = message.error.data?.details;
                    const errorMsg = details
                        ? `${message.error.message}: ${details}`
                        : message.error.message;
                    pending.reject(new Error(errorMsg));
                } else {
                    pending.resolve(message.result);
                }
            }
        } else if (message.method) {
            this.handleNotification(
                message.method,
                message.params as Record<string, unknown>,
            );
        }
    }

    private handleNotification(
        method: string,
        params: Record<string, unknown>,
    ): void {
        switch (method) {
            case "participantJoined":
                // Server sends participantId, client expects id
                this.emit("participantJoined", {
                    id: params.participantId as string,
                    metadata: (params.metadata as Record<string, unknown>) ?? {},
                    tracks: (params.tracks as TrackInfo[]) ?? [],
                } as ParticipantInfo);
                break;
            case "participantLeft":
                // Server sends participantId, client expects id
                this.emit("participantLeft", {
                    id: params.participantId as string,
                    reason: params.reason as string,
                });
                break;
            case "trackPublished":
                this.emit("trackPublished", {
                    publisherId: params.publisherId as string,
                    track: {
                        trackId: params.trackId as string,
                        kind: params.kind as "audio" | "video",
                        simulcast: params.simulcast as boolean,
                        metadata: (params.metadata as Record<string, unknown>) ?? {},
                    },
                });
                break;
            case "trackUnpublished":
                this.emit("trackUnpublished", {
                    publisherId: params.publisherId as string,
                    trackId: params.trackId as string,
                });
                break;
            case "offer":
                this.handleServerOffer(params as { sdp: string }).catch(
                    console.error,
                );
                break;
            case "candidate":
                this.handleServerCandidate(
                    params as {
                        candidate: string;
                        sdpMid: string;
                        sdpMLineIndex: number;
                    },
                ).catch(console.error);
                break;
            case "layerChanged":
                this.emit("layerChanged", params as unknown as LayerChangedEvent);
                break;
            case "error":
                this.emit("error", params as unknown as ServerError);
                break;
            case "reconnect":
                this.handleReconnectRequest(
                    params as { reason: string; retryAfterMs: number },
                );
                break;
        }
    }

    private async handleServerOffer(params: { sdp: string }): Promise<void> {
        if (!this.pc) return;

        console.log("Received server offer, sdp_length:", params.sdp.length);

        await this.pc.setRemoteDescription({
            type: "offer",
            sdp: params.sdp,
        });

        const answer = await this.pc.createAnswer();
        await this.pc.setLocalDescription(answer);

        console.log("Sending answer to server, sdp_length:", answer.sdp?.length);

        await this.sendRequest("answer", {
            sdp: this.pc.localDescription?.sdp,
        });

        // Process any queued ICE candidates
        await this.processPendingCandidates();

        console.log("Server offer handled successfully");
    }

    private async handleServerCandidate(params: {
        candidate: string;
        sdpMid: string;
        sdpMLineIndex: number;
    }): Promise<void> {
        if (!this.pc) return;

        // Queue candidates if remote description is not set yet
        if (!this.pc.remoteDescription) {
            console.log("Queuing ICE candidate (remote description not set yet)");
            this.pendingCandidates.push(params);
            return;
        }

        await this.pc.addIceCandidate({
            candidate: params.candidate,
            sdpMid: params.sdpMid,
            sdpMLineIndex: params.sdpMLineIndex,
        });
    }

    private async processPendingCandidates(): Promise<void> {
        if (!this.pc || this.pendingCandidates.length === 0) return;

        console.log(`Processing ${this.pendingCandidates.length} queued ICE candidates`);
        const candidates = [...this.pendingCandidates];
        this.pendingCandidates = [];

        for (const candidate of candidates) {
            try {
                await this.pc.addIceCandidate({
                    candidate: candidate.candidate,
                    sdpMid: candidate.sdpMid,
                    sdpMLineIndex: candidate.sdpMLineIndex,
                });
            } catch (err) {
                console.error("Failed to add queued ICE candidate:", err);
            }
        }
    }

    private handleReconnectRequest(params: {
        reason: string;
        retryAfterMs: number;
    }): void {
        console.log("Reconnect requested:", params.reason);
        setTimeout(() => {
            this.reconnect().catch(console.error);
        }, params.retryAfterMs);
    }

    private handleClose(code: number, reason: string): void {
        console.log("WebSocket closed:", code, reason);
        this.connectionState = "disconnected";
        this.emit("disconnected", reason || `Code: ${code}`);

        if (this.sessionId && this.config.reconnect) {
            this.reconnect().catch(console.error);
        }
    }

    private async reconnect(): Promise<void> {
        const config = this.config.reconnect!;

        while (this.reconnectAttempts < config.maxAttempts) {
            this.connectionState = "reconnecting";
            this.emit("reconnecting");

            const delay = Math.min(
                config.initialDelay *
                    Math.pow(config.factor, this.reconnectAttempts),
                config.maxDelay,
            );

            console.log(
                `Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts + 1})`,
            );

            await new Promise((resolve) => setTimeout(resolve, delay));

            try {
                this.ws = new WebSocket(this.config.url);

                await new Promise<void>((resolve, reject) => {
                    this.ws!.onopen = () => resolve();
                    this.ws!.onerror = () => reject(new Error("Connection failed"));
                });

                this.ws.onmessage = (event) => {
                    this.handleMessage(JSON.parse(event.data));
                };
                this.ws.onclose = (event) => {
                    this.handleClose(event.code, event.reason);
                };

                this.connectionState = "connected";
                this.reconnectAttempts = 0;
                this.emit("reconnected");
                return;
            } catch {
                this.reconnectAttempts++;
                console.error("Reconnect failed");
            }
        }

        console.error("Max reconnection attempts reached");
        this.connectionState = "disconnected";
    }
}
