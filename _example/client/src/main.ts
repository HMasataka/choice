type JSONValue = string | number | boolean | null | JSONObject | JSONArray;
interface JSONObject {
  [key: string]: JSONValue;
}
type JSONArray = JSONValue[];

import { els } from "@ui/dom";
import { log } from "@app/logger";
import { encodeVS, decodeVS } from "@signaling/vscodec";
import { resolveWsUrl, makeRequest } from "@signaling/rpc";
import { startLocalMedia, getLocalStream, stopLocalMedia } from "@webrtc/media";
import {
  ensurePublisherPC,
  setupPublisherTracks,
  addPublisherCandidate,
  closePublisher,
} from "@webrtc/publisher";
import {
  ensureSubscriberPC,
  createAnswerForOffer,
  flushOrBufferSubscriberCandidate,
  flushPendingSubscriberCandidates,
  closeSubscriber,
} from "@webrtc/subscriber";

// WebRTC config only (connections managed in modules)

const rtcConfig: RTCConfiguration = {
  iceServers: [{ urls: ["stun:stun.l.google.com:19302"] }],
};

// JSON-RPC over WS with VSCode framing
let rpcId = 1;
let ws: WebSocket | null = null;
let wsOpenPromise: Promise<void> | null = null;
let wsBuffer = ""; // バッファリング用
const pending = new Map<
  number,
  { resolve: (v: any) => void; reject: (e: any) => void }
>();

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
  ensureSubscriberPC(rtcConfig, (c) => sendCandidate("subscriber", c));
  const desc: RTCSessionDescriptionInit | null = params?.desc ?? params;
  if (!desc) throw new Error("missing offer");
  const localAnswer = await createAnswerForOffer(desc, els.trickle.checked);
  try {
    await rpcCall("answer", {
      session_id: els.sessionId.value,
      user_id: els.userId.value,
      answer: localAnswer as any,
    });
    log("sent subscriber answer");
  } catch (e: any) {
    log("answer rpc error", e.message || String(e));
    // keep buffering until a new offer arrives (state maintained in module)
  }
  await flushPendingSubscriberCandidates();
}

async function handleServerCandidate(params: any) {
  const type = params?.connection_type as "publisher" | "subscriber";
  const cand = params?.candidate as RTCIceCandidateInit | undefined;
  if (!cand || !type) return;
  if (type === "publisher") {
    await addPublisherCandidate(cand);
  } else {
    await flushOrBufferSubscriberCandidate(cand);
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
  const pc = ensurePublisherPC(rtcConfig, (c) => sendCandidate("publisher", c));
  let stream = getLocalStream();
  if (!stream) stream = await startLocalMedia();
  await setupPublisherTracks(stream);
  const offer = await pc.createOffer();
  await pc.setLocalDescription(offer);
  if (!els.trickle.checked) {
    await new Promise<void>((resolve) => {
      if (pc.iceGatheringState === "complete") return resolve();
      const onChange = () => {
        if (pc.iceGatheringState === "complete") {
          pc.removeEventListener("icegatheringstatechange", onChange);
          resolve();
        }
      };
      pc.addEventListener("icegatheringstatechange", onChange);
      setTimeout(() => resolve(), 2500);
    });
  }
  const res = await rpcCall<{ answer: RTCSessionDescriptionInit | null }>(
    "join",
    {
      session_id: els.sessionId.value,
      user_id: els.userId.value,
      offer: pc.localDescription as any,
    },
  );
  if (res?.answer) {
    await pc.setRemoteDescription(res.answer);
    log("publisher established");
  }
}

async function hangup() {
  closePublisher();
  closeSubscriber();
  stopLocalMedia();
  els.remoteVideo.srcObject = null;
  log("hung up");
}

els.btnStart.onclick = () =>
  startLocalMedia().catch((e) => log("start err", e.message));
els.btnJoin.onclick = () => join().catch((e) => log("join err", e.message));
els.btnHangup.onclick = () => {
  hangup();
};

log("Ready: 1) Start Camera 2) Join");
