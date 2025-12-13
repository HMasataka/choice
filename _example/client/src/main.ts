type JSONValue = string | number | boolean | null | JSONObject | JSONArray;
interface JSONObject {
  [key: string]: JSONValue;
}
type JSONArray = JSONValue[];

const els = {
  localVideo: document.getElementById("localVideo") as HTMLVideoElement,
  remoteVideo: document.getElementById("remoteVideo") as HTMLVideoElement,
  serverUrl: document.getElementById("serverUrl") as HTMLInputElement,
  sessionId: document.getElementById("sessionId") as HTMLInputElement,
  userId: document.getElementById("userId") as HTMLInputElement,
  btnStart: document.getElementById("btnStart") as HTMLButtonElement,
  btnJoin: document.getElementById("btnJoin") as HTMLButtonElement,
  btnHangup: document.getElementById("btnHangup") as HTMLButtonElement,
  trickle: document.getElementById("trickle") as HTMLInputElement,
  offerOut: document.getElementById("offerOut") as HTMLTextAreaElement,
  answerIn: document.getElementById("answerIn") as HTMLTextAreaElement,
  serverOffer: document.getElementById("serverOffer") as HTMLTextAreaElement,
  btnCreateAnswer: document.getElementById(
    "btnCreateAnswer",
  ) as HTMLButtonElement,
  btnSendAnswer: document.getElementById("btnSendAnswer") as HTMLButtonElement,
  answerOut: document.getElementById("answerOut") as HTMLTextAreaElement,
  localCandidate: document.getElementById(
    "localCandidate",
  ) as HTMLTextAreaElement,
  serverCandidate: document.getElementById(
    "serverCandidate",
  ) as HTMLTextAreaElement,
  btnAddServerCandidate: document.getElementById(
    "btnAddServerCandidate",
  ) as HTMLButtonElement,
  btnSendServerCandidate: document.getElementById(
    "btnSendServerCandidate",
  ) as HTMLButtonElement,
  log: document.getElementById("log") as HTMLPreElement,
};

function log(...args: any[]) {
  const line = args
    .map((a) => (typeof a === "string" ? a : JSON.stringify(a)))
    .join(" ");
  els.log.textContent += line + "\n";
  els.log.scrollTop = els.log.scrollHeight;
}

let pc: RTCPeerConnection | null = null;
let localStream: MediaStream | null = null;
let lastLocalCandidate: RTCIceCandidateInit | null = null;
let publisherReady = false;

const rtcConfig: RTCConfiguration = {
  iceServers: [{ urls: ["stun:stun.l.google.com:19302"] }],
};

function ensurePC() {
  if (pc) return pc;
  pc = new RTCPeerConnection(rtcConfig);

  pc.onicecandidate = (ev) => {
    if (ev.candidate) {
      lastLocalCandidate = ev.candidate.toJSON();
      els.localCandidate.value = JSON.stringify(lastLocalCandidate);
      if (els.trickle.checked && publisherReady) {
        sendCandidate("publisher", lastLocalCandidate).catch((e) =>
          log("send candidate error", e.message),
        );
      }
    }
  };

  pc.onicegatheringstatechange = () => {
    log("iceGatheringState:", pc?.iceGatheringState);
  };

  pc.onconnectionstatechange = () => {
    log("connectionState:", pc?.connectionState);
  };

  pc.ontrack = (ev) => {
    const [stream] = ev.streams;
    els.remoteVideo.srcObject = stream;
  };

  return pc;
}

async function startCamera() {
  ensurePC();
  localStream = await navigator.mediaDevices.getUserMedia({
    video: true,
    audio: true,
  });
  els.localVideo.srcObject = localStream;
  localStream.getTracks().forEach((t) => pc!.addTrack(t, localStream!));
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

let rpcId = 1;
let ws: WebSocket | null = null;
let wsOpenPromise: Promise<void> | null = null;
const pending = new Map<number, { resolve: (v: any) => void; reject: (e: any) => void }>();

function resolveWsUrl(path: string): string {
  try {
    // If absolute ws(s) URL
    if (path.startsWith("ws://") || path.startsWith("wss://")) return path;
    // If absolute http(s) URL, convert to ws(s)
    if (path.startsWith("http://") || path.startsWith("https://")) {
      const u = new URL(path);
      u.protocol = u.protocol.replace("http", "ws");
      return u.toString();
    }
    // Relative path → same host/port
    const origin = new URL(window.location.origin);
    origin.protocol = origin.protocol.replace("http", "ws");
    return origin.origin + path;
  } catch {
    return path;
  }
}

function encodeVSCodeMessage(obj: any): string {
  const json = JSON.stringify(obj);
  const bytes = new TextEncoder().encode(json).length;
  return `Content-Length: ${bytes}\r\n\r\n${json}`;
}

function* decodeVSCodeMessages(text: string): Generator<string> {
  let buf = text;
  while (true) {
    const headerIdx = buf.indexOf("\r\n\r\n");
    if (headerIdx === -1) return;
    const header = buf.slice(0, headerIdx);
    const m = /Content-Length:\s*(\d+)/i.exec(header);
    if (!m) return;
    const len = parseInt(m[1], 10);
    const start = headerIdx + 4;
    const end = start + len;
    if (buf.length < end) return; // incomplete
    const json = buf.slice(start, end);
    yield json;
    buf = buf.slice(end);
    if (!buf) return;
  }
}

function ensureWS(): Promise<void> {
  if (ws && ws.readyState === WebSocket.OPEN) return Promise.resolve();
  if (ws && ws.readyState === WebSocket.CONNECTING && wsOpenPromise) return wsOpenPromise;

  const url = resolveWsUrl(els.serverUrl.value || "/ws");
  ws = new WebSocket(url);

  wsOpenPromise = new Promise<void>((resolve, reject) => {
    ws!.onopen = () => {
      log("WebSocket connected:", url);
      resolve();
    };
    ws!.onerror = (ev) => {
      reject(new Error("WebSocket error"));
    };
  });

  ws.onmessage = (ev) => {
    try {
      const data = String(ev.data);
      for (const json of decodeVSCodeMessages(data)) {
        const msg = JSON.parse(json);
        if (msg && Object.prototype.hasOwnProperty.call(msg, "id")) {
          const entry = pending.get(msg.id);
          if (entry) {
            pending.delete(msg.id);
            if (Object.prototype.hasOwnProperty.call(msg, "result")) {
              entry.resolve(msg.result);
            } else if (Object.prototype.hasOwnProperty.call(msg, "error")) {
              entry.reject(new Error(typeof msg.error === "string" ? msg.error : JSON.stringify(msg.error)));
            } else {
              entry.resolve(undefined);
            }
          }
        } else if (msg && msg.method) {
          log("Notification:", msg.method, msg.params ?? "");
        }
      }
    } catch (e: any) {
      log("WS message parse error:", e.message || String(e));
    }
  };

  ws.onclose = () => {
    log("WebSocket closed");
    // Reject all pending
    for (const [id, p] of pending) {
      p.reject(new Error("connection closed"));
    }
    pending.clear();
    wsOpenPromise = null;
  };

  return wsOpenPromise;
}

async function rpcCall<T = any>(method: string, params: JSONObject): Promise<T> {
  await ensureWS();
  const id = rpcId++;
  const payload = { jsonrpc: "2.0", method, params, id };
  return new Promise<T>((resolve, reject) => {
    pending.set(id, { resolve, reject });
    try {
      ws!.send(encodeVSCodeMessage(payload));
    } catch (e) {
      pending.delete(id);
      reject(e);
    }
  });
}

async function join() {
  ensurePC();
  const offer = await pc!.createOffer({
    offerToReceiveAudio: false,
    offerToReceiveVideo: false,
  });
  await pc!.setLocalDescription(offer);
  await waitIceComplete(pc!);
  const localDesc = pc!.localDescription!;
  els.offerOut.value = JSON.stringify(localDesc);

  const result = await rpcCall<{ answer: RTCSessionDescriptionInit | null }>(
    "join",
    {
      session_id: els.sessionId.value,
      user_id: els.userId.value,
      offer: localDesc as any,
    },
  );

  if (result?.answer) {
    els.answerIn.value = JSON.stringify(result.answer);
    await pc!.setRemoteDescription(result.answer);
    publisherReady = true;
    log("Join answered and remoteDescription set.");
  } else {
    log("Join response had no answer (server not implemented?).");
  }
}

async function createAnswerFromServerOffer() {
  ensurePC();
  const offer = JSON.parse(
    els.serverOffer.value || "null",
  ) as RTCSessionDescriptionInit;
  if (!offer) throw new Error("No server offer JSON");
  await pc!.setRemoteDescription(offer);
  const answer = await pc!.createAnswer();
  await pc!.setLocalDescription(answer);
  await waitIceComplete(pc!);
  els.answerOut.value = JSON.stringify(pc!.localDescription);
  log("Created local Answer for server Offer.");
}

async function sendAnswerRPC() {
  const answer = els.answerOut.value
    ? (JSON.parse(els.answerOut.value) as RTCSessionDescriptionInit)
    : null;
  if (!answer) throw new Error("No local Answer to send");
  await rpcCall("answer", {
    session_id: els.sessionId.value,
    user_id: els.userId.value,
    answer: answer as any,
  });
  log("Sent Answer via RPC.");
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
  log("Sent candidate", type);
}

async function addServerCandidate() {
  const c = els.serverCandidate.value
    ? (JSON.parse(els.serverCandidate.value) as RTCIceCandidateInit)
    : null;
  if (!c) throw new Error("Paste a server candidate JSON");
  await pc!.addIceCandidate(c);
  log("Added remote candidate to PC.");
}

async function hangup() {
  pc?.getSenders().forEach((s) => s.track && s.track.stop());
  pc?.close();
  pc = null;
  localStream = null;
  publisherReady = false;
  els.localVideo.srcObject = null;
  els.remoteVideo.srcObject = null;
  log("Hung up.");
}

els.btnStart.onclick = () =>
  startCamera().catch((e) => log("start error", e.message));
els.btnJoin.onclick = () => join().catch((e) => log("join error", e.message));
els.btnHangup.onclick = () => hangup();
els.btnCreateAnswer.onclick = () =>
  createAnswerFromServerOffer().catch((e) => log("createAnswer error", e.message));
els.btnSendAnswer.onclick = () =>
  sendAnswerRPC().catch((e) => log("sendAnswer error", e.message));
els.btnAddServerCandidate.onclick = () =>
  addServerCandidate().catch((e) => log("add candidate error", e.message));
els.btnSendServerCandidate.onclick = () => {
  if (lastLocalCandidate) {
    sendCandidate("publisher", lastLocalCandidate).catch((e) =>
      log("send candidate error", e.message),
    );
  } else {
    log("No local candidate to send.");
  }
};
els.btnCreateAnswer.onclick = () =>
  createAnswerFromServerOffer().catch((e) =>
    log("answer create error", e.message),
  );
els.btnSendAnswer.onclick = () =>
  sendAnswerRPC().catch((e) => log("answer rpc error", e.message));
els.btnAddServerCandidate.onclick = () =>
  addServerCandidate().catch((e) => log("candidate add error", e.message));
els.btnSendServerCandidate.onclick = () => {
  if (!lastLocalCandidate) return log("No local candidate yet");
  sendCandidate("publisher", lastLocalCandidate).catch((e) =>
    log("candidate send error", e.message),
  );
};

log("Ready. 1) Start Camera 2) Join");
