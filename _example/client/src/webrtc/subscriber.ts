import { elements } from "@ui/dom";
import { log } from "@app/logger";

let subscriber: RTCPeerConnection | null = null; // receives remote media
let subRemoteDescSet = false;
const subPendingRemoteCandidates: RTCIceCandidateInit[] = [];

export function getSubscriberPC(): RTCPeerConnection | null {
  return subscriber;
}

export function closeSubscriber() {
  try {
    subscriber?.close();
  } catch {}
  subscriber = null;
  subRemoteDescSet = false;
  subPendingRemoteCandidates.length = 0;
  try {
    elements.remoteVideo.srcObject = null;
  } catch {}
}

export function ensureSubscriberPC(
  rtcConfig: RTCConfiguration,
  onCandidate: (cand: RTCIceCandidateInit) => void | Promise<void>,
): RTCPeerConnection {
  if (subscriber) return subscriber;
  subscriber = new RTCPeerConnection(rtcConfig);
  subRemoteDescSet = false;
  subPendingRemoteCandidates.length = 0;

  try {
    subscriber.addTransceiver("video", { direction: "recvonly" });
  } catch {}
  try {
    subscriber.addTransceiver("audio", { direction: "recvonly" });
  } catch {}

  subscriber.onicecandidate = (ev) => {
    if (ev.candidate && elements.trickle.checked) {
      const cand = ev.candidate.toJSON();
      Promise.resolve(onCandidate(cand)).catch((e) =>
        log("sub cand err", e?.message || String(e)),
      );
    }
  };
  subscriber.onicegatheringstatechange = () =>
    log("sub gather:", subscriber!.iceGatheringState);
  subscriber.onconnectionstatechange = () =>
    log("sub conn:", subscriber!.connectionState);
  subscriber.ontrack = (ev) => {
    const [stream] = ev.streams;
    log("sub ontrack: kind=", ev.track.kind, "id=", ev.track.id);
    elements.remoteVideo.srcObject = stream;
    elements.remoteVideo.muted = true;
    (elements.remoteVideo as any).playsInline = true;
    elements.remoteVideo.onloadedmetadata = () => {
      try {
        const v =
          (
            elements.remoteVideo.srcObject as MediaStream | null
          )?.getVideoTracks().length || 0;
        const a =
          (
            elements.remoteVideo.srcObject as MediaStream | null
          )?.getAudioTracks().length || 0;
        log("remote loadedmetadata: vtracks=", v, "atracks=", a);
      } catch {}
    };
    elements.remoteVideo.onplaying = () => log("remote playing");
    elements.remoteVideo
      .play()
      .catch((e) => log("remote play() failed(sub)", e?.message || String(e)));
  };

  return subscriber;
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

export async function createAnswerForOffer(
  desc: RTCSessionDescriptionInit,
  trickle: boolean,
): Promise<RTCSessionDescriptionInit> {
  if (!subscriber) throw new Error("subscriber PC not initialized");
  await subscriber.setRemoteDescription(desc);
  const answer = await subscriber.createAnswer();
  await subscriber.setLocalDescription(answer);
  subRemoteDescSet = true;

  await waitForLocalUfrag(subscriber);
  if (!trickle) await waitIceComplete(subscriber);
  const sdp = subscriber.localDescription?.sdp || "";
  log("sub answer ufrag:", /a=ice-ufrag:/i.test(sdp) ? "present" : "missing");
  return subscriber.localDescription as RTCSessionDescriptionInit;
}

export async function flushOrBufferSubscriberCandidate(
  cand: RTCIceCandidateInit,
) {
  if (
    !subscriber ||
    !subRemoteDescSet ||
    !subscriber.remoteDescription ||
    !subscriber.localDescription ||
    !hasIceUfrag(subscriber.localDescription)
  ) {
    subPendingRemoteCandidates.push(cand);
    return;
  }
  try {
    await subscriber.addIceCandidate(cand);
  } catch (e: any) {
    log("sub cand err", e?.message || String(e));
  }
}

export async function flushPendingSubscriberCandidates() {
  while (subPendingRemoteCandidates.length) {
    const c = subPendingRemoteCandidates.shift()!;
    try {
      await subscriber!.addIceCandidate(c);
    } catch (e: any) {
      log("flush sub cand err", e?.message || String(e));
    }
  }
}
