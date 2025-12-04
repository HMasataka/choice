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
      if (els.trickle.checked) {
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
async function rpcCall<T = any>(
  method: string,
  params: JSONObject,
): Promise<T> {
  const payload = { method, params: [params], id: rpcId++ };
  const res = await fetch(els.serverUrl.value, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!res.ok) throw new Error(`RPC ${method} failed: ${res.status}`);
  const body = (await res.json()) as { result?: T; error?: any; id?: number };
  if (body.error) throw new Error(`RPC error: ${JSON.stringify(body.error)}`);
  return body.result as T;
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
    "SignalingServer.Join",
    {
      session_id: els.sessionId.value,
      user_id: els.userId.value,
      offer: localDesc as any,
    },
  );

  if (result?.answer) {
    els.answerIn.value = JSON.stringify(result.answer);
    await pc!.setRemoteDescription(result.answer);
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
  await rpcCall("SignalingServer.Answer", { answer: answer as any });
  log("Sent Answer via RPC.");
}

async function sendCandidate(
  type: "publisher" | "subscriber",
  cand: RTCIceCandidateInit,
) {
  await rpcCall("SignalingServer.Candidate", {
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
  els.localVideo.srcObject = null;
  els.remoteVideo.srcObject = null;
  log("Hung up.");
}

els.btnStart.onclick = () =>
  startCamera().catch((e) => log("start error", e.message));
els.btnJoin.onclick = () => join().catch((e) => log("join error", e.message));
els.btnHangup.onclick = () => hangup();
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
