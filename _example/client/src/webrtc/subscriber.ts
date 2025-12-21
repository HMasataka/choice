import { els } from "@ui/dom";
import { log } from "@app/logger";

let pcSub: RTCPeerConnection | null = null; // receives remote media
let subRemoteDescSet = false;
const subPendingRemoteCandidates: RTCIceCandidateInit[] = [];

export function getSubscriberPC(): RTCPeerConnection | null {
  return pcSub;
}

export function closeSubscriber() {
  try {
    pcSub?.close();
  } catch {}
  pcSub = null;
  subRemoteDescSet = false;
  subPendingRemoteCandidates.length = 0;
  try {
    els.remoteVideo.srcObject = null;
  } catch {}
}

export function ensureSubscriberPC(
  rtcConfig: RTCConfiguration,
  onCandidate: (cand: RTCIceCandidateInit) => void | Promise<void>,
): RTCPeerConnection {
  if (pcSub) return pcSub;
  pcSub = new RTCPeerConnection(rtcConfig);
  subRemoteDescSet = false;
  subPendingRemoteCandidates.length = 0;

  try {
    pcSub.addTransceiver("video", { direction: "recvonly" });
  } catch {}
  try {
    pcSub.addTransceiver("audio", { direction: "recvonly" });
  } catch {}

  pcSub.onicecandidate = (ev) => {
    if (ev.candidate && els.trickle.checked) {
      const cand = ev.candidate.toJSON();
      Promise.resolve(onCandidate(cand)).catch((e) =>
        log("sub cand err", e?.message || String(e)),
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
  if (!pcSub) throw new Error("subscriber PC not initialized");
  await pcSub.setRemoteDescription(desc);
  const answer = await pcSub.createAnswer();
  await pcSub.setLocalDescription(answer);
  subRemoteDescSet = true;

  await waitForLocalUfrag(pcSub);
  if (!trickle) await waitIceComplete(pcSub);
  const sdp = pcSub.localDescription?.sdp || "";
  log("sub answer ufrag:", /a=ice-ufrag:/i.test(sdp) ? "present" : "missing");
  return pcSub.localDescription as RTCSessionDescriptionInit;
}

export async function flushOrBufferSubscriberCandidate(
  cand: RTCIceCandidateInit,
) {
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
    await pcSub.addIceCandidate(cand);
  } catch (e: any) {
    log("sub cand err", e?.message || String(e));
  }
}

export async function flushPendingSubscriberCandidates() {
  while (subPendingRemoteCandidates.length) {
    const c = subPendingRemoteCandidates.shift()!;
    try {
      await pcSub!.addIceCandidate(c);
    } catch (e: any) {
      log("flush sub cand err", e?.message || String(e));
    }
  }
}
