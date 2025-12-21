type JSONValue = string | number | boolean | null | JSONObject | JSONArray;
interface JSONObject {
  [key: string]: JSONValue;
}
type JSONArray = JSONValue[];

import { els } from "@ui/dom";
import { log } from "@app/logger";
import { encodeVS, decodeVS } from "@signaling/vscodec";
import { resolveWsUrl, makeRequest } from "@signaling/rpc";

// WebRTC state
let pcPub: RTCPeerConnection | null = null; // sends local media
let pcSub: RTCPeerConnection | null = null; // receives remote media
let localStream: MediaStream | null = null;
let subRemoteDescSet = false;
const subPendingRemoteCandidates: RTCIceCandidateInit[] = [];

const rtcConfig: RTCConfiguration = {
  iceServers: [{ urls: ["stun:stun.l.google.com:19302"] }],
};

function ensurePublisherPC(): RTCPeerConnection {
  if (pcPub) return pcPub;
  pcPub = new RTCPeerConnection(rtcConfig);

  pcPub.onicecandidate = (ev) => {
    if (ev.candidate && els.trickle.checked) {
      const cand = ev.candidate.toJSON();
      sendCandidate("publisher", cand).catch((e) =>
        log("pub cand err", e.message),
      );
    }
  };
  pcPub.onicegatheringstatechange = () =>
    log("pub gather:", pcPub!.iceGatheringState);
  pcPub.onconnectionstatechange = () =>
    log("pub conn:", pcPub!.connectionState);
  pcPub.ontrack = (ev) => {
    const [stream] = ev.streams;
    log("pub ontrack: kind=", ev.track.kind, "id=", ev.track.id);
    els.remoteVideo.srcObject = stream;
    // Avoid autoplay policies blocking playback
    els.remoteVideo.muted = true;
    (els.remoteVideo as any).playsInline = true;
    els.remoteVideo
      .play()
      .catch((e) => log("remote play() failed(pub)", e?.message || String(e)));
  };
  return pcPub;
}

function ensureSubscriberPC(): RTCPeerConnection {
  if (pcSub) return pcSub;
  pcSub = new RTCPeerConnection(rtcConfig);
  subRemoteDescSet = false;
  subPendingRemoteCandidates.length = 0;

  // Ensure we have recv transceivers so m-lines/ICE creds are present
  try {
    pcSub.addTransceiver("video", { direction: "recvonly" });
  } catch {}
  try {
    pcSub.addTransceiver("audio", { direction: "recvonly" });
  } catch {}

  pcSub.onicecandidate = (ev) => {
    if (ev.candidate && els.trickle.checked) {
      const cand = ev.candidate.toJSON();
      sendCandidate("subscriber", cand).catch((e) =>
        log("sub cand err", e.message),
      );
    }
  };
  pcSub.onicegatheringstatechange = () =>
    log("sub gather:", pcSub!.iceGatheringState);
  pcSub.onconnectionstatechange = () =>
    log("sub conn:", pcSub!.connectionState);
  pcSub.ontrack = (ev) => {
    const [stream] = ev.streams;
    log("sub ontrack: kind=", ev.track.kind, "id=", ev.track.id);
    els.remoteVideo.srcObject = stream;
    els.remoteVideo.muted = true;
    (els.remoteVideo as any).playsInline = true;
    els.remoteVideo.onloadedmetadata = () => {
      try {
        const v =
          (els.remoteVideo.srcObject as MediaStream | null)?.getVideoTracks()
            .length || 0;
        const a =
          (els.remoteVideo.srcObject as MediaStream | null)?.getAudioTracks()
            .length || 0;
        log("remote loadedmetadata: vtracks=", v, "atracks=", a);
      } catch {}
    };
    els.remoteVideo.onplaying = () => log("remote playing");
    els.remoteVideo
      .play()
      .catch((e) => log("remote play() failed(sub)", e?.message || String(e)));
  };
  return pcSub;
}

function hasIceUfrag(d?: RTCSessionDescription | null): boolean {
  return !!(d?.sdp && /a=ice-ufrag:/i.test(d.sdp));
}

function waitForLocalUfrag(
  p: RTCPeerConnection,
  timeoutMs = 2000,
): Promise<void> {
  if (hasIceUfrag(p.localDescription)) return Promise.resolve();
  return new Promise((resolve) => {
    const start = Date.now();
    const check = () => {
      if (hasIceUfrag(p.localDescription) || Date.now() - start > timeoutMs) {
        return resolve();
      }
      setTimeout(check, 50);
    };
    check();
  });
}

async function startCamera() {
  ensurePublisherPC();
  localStream = await navigator.mediaDevices.getUserMedia({
    video: {
      width: { ideal: 1280 },
      height: { ideal: 720 },
      frameRate: { ideal: 30 },
    },
    audio: true,
  });
  els.localVideo.srcObject = localStream;

  // Audio は通常どおり addTrack
  const audioTrack = localStream.getAudioTracks()[0];
  if (audioTrack) pcPub!.addTrack(audioTrack, localStream!);

  // Video は simulcast: まず sendonly transceiver を作成し encodings を指定、
  // その後 replaceTrack で実トラックをアタッチ（Safari 等の互換性向上）
  const videoTrack = localStream.getVideoTracks()[0];
  if (videoTrack) {
    try {
      const vtx = pcPub!.addTransceiver("video", {
        direction: "sendonly",
        sendEncodings: [
          {
            rid: "q",
            scaleResolutionDownBy: 4.0,
            maxBitrate: 300_000,
            maxFramerate: 30,
          },
          {
            rid: "h",
            scaleResolutionDownBy: 2.0,
            maxBitrate: 900_000,
            maxFramerate: 30,
          },
        ],
      });
      // Prefer VP8 for simulcast if available
      try {
        // @ts-ignore experimental
        const caps = RTCRtpSender.getCapabilities?.("video");
        if (caps && Array.isArray(caps.codecs)) {
          const vp8 = caps.codecs.filter((c: any) => /VP8/i.test(c.mimeType));
          const others = caps.codecs.filter(
            (c: any) => !/VP8/i.test(c.mimeType),
          );
          // @ts-ignore experimental
          if (vp8.length && vtx.setCodecPreferences) {
            // @ts-ignore experimental
            vtx.setCodecPreferences([...vp8, ...others]);
            log("setCodecPreferences: prefer VP8");
          }
        }
      } catch {}
      await vtx.sender.replaceTrack(videoTrack);
      // Ensure msid continuity for the video sender when using replaceTrack
      try {
        // @ts-ignore experimental
        vtx.sender.setStreams?.(localStream);
      } catch {}
      log("enabled simulcast encodings (q/h) with replaceTrack");
    } catch (e: any) {
      log(
        "simulcast setup failed, fallback to single stream:",
        e?.message || String(e),
      );
      pcPub!.addTrack(videoTrack, localStream!);
    }
  }
}

function waitIceComplete(p: RTCPeerConnection) {
  return new Promise<void>((resolve) => {
    if (p.iceGatheringState === "complete") return resolve();
    const onChange = () => {
      if (p.iceGatheringState === "complete") {
        p.removeEventListener("icegatheringstatechange", onChange);
        resolve();
      }
    };
    p.addEventListener("icegatheringstatechange", onChange);
    setTimeout(() => resolve(), 2500);
  });
}

// JSON-RPC over WS with VSCode framing
let rpcId = 1;
let ws: WebSocket | null = null;
let wsOpenPromise: Promise<void> | null = null;
let wsBuffer = ""; // バッファリング用
const pending = new Map<
  number,
  { resolve: (v: any) => void; reject: (e: any) => void }
>();

// moved: resolveWsUrl, encodeVS, decodeVS to @signaling modules

function ensureWS(): Promise<void> {
  if (ws && ws.readyState === WebSocket.OPEN) return Promise.resolve();
  if (ws && ws.readyState === WebSocket.CONNECTING && wsOpenPromise)
    return wsOpenPromise;
  const url = resolveWsUrl(els.serverUrl.value || "/ws");
  ws = new WebSocket(url);
  wsBuffer = ""; // 新しい接続でバッファをクリア
  wsOpenPromise = new Promise<void>((resolve, reject) => {
    ws!.onopen = () => {
      log("WS connected", url);
      resolve();
    };
    ws!.onerror = () => {
      reject(new Error("WebSocket error"));
    };
  });
  ws.onmessage = (ev) => {
    try {
      const data = String(ev.data);
      log("WS RAW message received:", data.substring(0, 100));

      // バッファに追加
      wsBuffer += data;
      log("WS buffer length:", wsBuffer.length);

      // バッファからメッセージをパース
      const { messages, remaining } = decodeVS(wsBuffer);
      wsBuffer = remaining; // 残りをバッファに保存
      log(
        "WS parsed",
        messages.length,
        "messages, remaining:",
        remaining.length,
      );

      for (const json of messages) {
        log("WS parsed JSON:", json.substring(0, 100));
        const msg = JSON.parse(json);
        log("WS msg object:", JSON.stringify(msg).substring(0, 200));
        if (Object.prototype.hasOwnProperty.call(msg, "id")) {
          const p = pending.get(msg.id);
          if (!p) continue;
          pending.delete(msg.id);
          if (Object.prototype.hasOwnProperty.call(msg, "result"))
            p.resolve(msg.result);
          else if (Object.prototype.hasOwnProperty.call(msg, "error"))
            p.reject(
              new Error(
                typeof msg.error === "string"
                  ? msg.error
                  : JSON.stringify(msg.error),
              ),
            );
          else p.resolve(undefined);
        } else if (msg && msg.method) {
          log("WS notification method:", msg.method);
          if (msg.method === "offer")
            handleServerOffer(msg.params).catch((e) =>
              log("offer err", e.message),
            );
          else if (msg.method === "candidate")
            handleServerCandidate(msg.params).catch((e) =>
              log("cand err", e.message),
            );
        }
      }
    } catch (e: any) {
      log("WS parse err", e.message || String(e));
    }
  };
  ws.onclose = () => {
    log("WS closed");
    wsBuffer = ""; // バッファをクリア
    for (const [, p] of pending) p.reject(new Error("closed"));
    pending.clear();
    wsOpenPromise = null;
  };
  return wsOpenPromise;
}

async function rpcCall<T = any>(
  method: string,
  params: JSONObject,
): Promise<T> {
  await ensureWS();
  const id = rpcId++;
  const payload = makeRequest(id, method, params);
  return new Promise<T>((resolve, reject) => {
    pending.set(id, { resolve, reject });
    try {
      ws!.send(encodeVS(payload));
    } catch (e) {
      pending.delete(id);
      reject(e);
    }
  });
}

async function handleServerOffer(params: any) {
  ensureSubscriberPC();
  const desc: RTCSessionDescriptionInit | null = params?.desc ?? params;
  if (!desc) throw new Error("missing offer");
  await pcSub!.setRemoteDescription(desc);
  const answer = await pcSub!.createAnswer();
  await pcSub!.setLocalDescription(answer);
  subRemoteDescSet = true;
  // Ensure local SDP includes ICE creds before sending
  await waitForLocalUfrag(pcSub!);
  if (!els.trickle.checked) await waitIceComplete(pcSub!);
  const sdp = pcSub!.localDescription?.sdp || "";
  log("sub answer ufrag:", /a=ice-ufrag:/i.test(sdp) ? "present" : "missing");
  try {
    await rpcCall("answer", {
      session_id: els.sessionId.value,
      user_id: els.userId.value,
      answer: pcSub!.localDescription as any,
    });
    log("sent subscriber answer");
  } catch (e: any) {
    log("answer rpc error", e.message || String(e));
    // keep buffering until a new offer arrives
    subRemoteDescSet = false;
  }
  // apply queued remote candidates after answering
  while (subPendingRemoteCandidates.length) {
    const c = subPendingRemoteCandidates.shift()!;
    try {
      await pcSub!.addIceCandidate(c);
    } catch (e: any) {
      log("flush sub cand err", e.message || String(e));
    }
  }
}

async function handleServerCandidate(params: any) {
  const type = params?.connection_type as "publisher" | "subscriber";
  const cand = params?.candidate as RTCIceCandidateInit | undefined;
  if (!cand || !type) return;
  if (type === "publisher") {
    if (!pcPub) return;
    await pcPub.addIceCandidate(cand);
  } else {
    // buffer until we have both remote and local descriptions on subscriber
    if (
      !pcSub ||
      !subRemoteDescSet ||
      !pcSub.remoteDescription ||
      !pcSub.localDescription ||
      !hasIceUfrag(pcSub.localDescription)
    ) {
      subPendingRemoteCandidates.push(cand);
      return;
    }
    try {
      await pcSub!.addIceCandidate(cand);
    } catch (e: any) {
      log("sub cand err", e.message || String(e));
    }
  }
}

async function sendCandidate(
  type: "publisher" | "subscriber",
  cand: RTCIceCandidateInit,
) {
  await rpcCall("candidate", {
    session_id: els.sessionId.value,
    user_id: els.userId.value,
    connection_type: type,
    candidate: cand as any,
  });
}

async function join() {
  ensurePublisherPC();
  if (!localStream) await startCamera();
  const offer = await pcPub!.createOffer();
  await pcPub!.setLocalDescription(offer);
  if (!els.trickle.checked) await waitIceComplete(pcPub!);
  const res = await rpcCall<{ answer: RTCSessionDescriptionInit | null }>(
    "join",
    {
      session_id: els.sessionId.value,
      user_id: els.userId.value,
      offer: pcPub!.localDescription as any,
    },
  );
  if (res?.answer) {
    await pcPub!.setRemoteDescription(res.answer);
    log("publisher established");
  }
}

async function hangup() {
  pcPub?.getSenders().forEach((s) => s.track?.stop());
  pcPub?.close();
  pcPub = null;
  pcSub?.close();
  pcSub = null;
  subRemoteDescSet = false;
  subPendingRemoteCandidates.length = 0;
  if (localStream) {
    localStream.getTracks().forEach((t) => t.stop());
  }
  localStream = null;
  els.localVideo.srcObject = null;
  els.remoteVideo.srcObject = null;
  log("hung up");
}

els.btnStart.onclick = () =>
  startCamera().catch((e) => log("start err", e.message));
els.btnJoin.onclick = () => join().catch((e) => log("join err", e.message));
els.btnHangup.onclick = () => {
  hangup();
};

log("Ready: 1) Start Camera 2) Join");
