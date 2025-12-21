import { els } from "@ui/dom";
import { log } from "@app/logger";

let pcPub: RTCPeerConnection | null = null;

export function getPublisherPC(): RTCPeerConnection | null {
  return pcPub;
}

export function closePublisher() {
  try {
    pcPub?.getSenders().forEach((s) => s.track?.stop());
    pcPub?.close();
  } catch {}
  pcPub = null;
}

export function ensurePublisherPC(
  rtcConfig: RTCConfiguration,
  onCandidate: (cand: RTCIceCandidateInit) => void | Promise<void>,
): RTCPeerConnection {
  if (pcPub) return pcPub;
  pcPub = new RTCPeerConnection(rtcConfig);

  pcPub.onicecandidate = (ev) => {
    if (ev.candidate && els.trickle.checked) {
      const cand = ev.candidate.toJSON();
      Promise.resolve(onCandidate(cand)).catch((e) =>
        log("pub cand err", e?.message || String(e)),
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
    els.remoteVideo.muted = true;
    (els.remoteVideo as any).playsInline = true;
    els.remoteVideo
      .play()
      .catch((e) => log("remote play() failed(pub)", e?.message || String(e)));
  };
  return pcPub;
}

export async function setupPublisherTracks(stream: MediaStream) {
  if (!pcPub) throw new Error("publisher PC not initialized");
  const audioTrack = stream.getAudioTracks()[0];
  if (audioTrack) pcPub.addTrack(audioTrack, stream);

  const videoTrack = stream.getVideoTracks()[0];
  if (!videoTrack) return;
  try {
    const vtx = pcPub.addTransceiver("video", {
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
    try {
      // @ts-ignore experimental
      const caps = RTCRtpSender.getCapabilities?.("video");
      if (caps && Array.isArray(caps.codecs)) {
        const vp8 = caps.codecs.filter((c: any) => /VP8/i.test(c.mimeType));
        const others = caps.codecs.filter((c: any) => !/VP8/i.test(c.mimeType));
        // @ts-ignore experimental
        if (vp8.length && vtx.setCodecPreferences) {
          // @ts-ignore experimental
          vtx.setCodecPreferences([...vp8, ...others]);
          log("setCodecPreferences: prefer VP8");
        }
      }
    } catch {}
    await vtx.sender.replaceTrack(videoTrack);
    try {
      // @ts-ignore experimental
      vtx.sender.setStreams?.(stream);
    } catch {}
    log("enabled simulcast encodings (q/h) with replaceTrack");
  } catch (e: any) {
    log(
      "simulcast setup failed, fallback to single stream:",
      e?.message || String(e),
    );
    pcPub.addTrack(videoTrack, stream);
  }
}

export async function addPublisherCandidate(cand: RTCIceCandidateInit) {
  if (!pcPub) return;
  await pcPub.addIceCandidate(cand);
}
