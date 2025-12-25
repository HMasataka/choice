import { elements } from "@ui/dom";
import { log } from "@app/logger";

let publisher: RTCPeerConnection | null = null;

export function getPublisherPC(): RTCPeerConnection | null {
  return publisher;
}

export function closePublisher() {
  try {
    publisher?.getSenders().forEach((s) => s.track?.stop());
    publisher?.close();
  } catch {}
  publisher = null;
}

export function ensurePublisherPC(
  rtcConfig: RTCConfiguration,
  onCandidate: (cand: RTCIceCandidateInit) => void | Promise<void>,
): RTCPeerConnection {
  if (publisher) return publisher;
  publisher = new RTCPeerConnection(rtcConfig);

  publisher.onicecandidate = (ev) => {
    if (ev.candidate && elements.trickle.checked) {
      const cand = ev.candidate.toJSON();
      Promise.resolve(onCandidate(cand)).catch((e) =>
        log("pub cand err", e?.message || String(e)),
      );
    }
  };
  publisher.onicegatheringstatechange = () =>
    log("pub gather:", publisher!.iceGatheringState);
  publisher.onconnectionstatechange = () =>
    log("pub conn:", publisher!.connectionState);
  publisher.ontrack = (ev) => {
    const [stream] = ev.streams;
    log("pub ontrack: kind=", ev.track.kind, "id=", ev.track.id);
    elements.remoteVideo.srcObject = stream;
    elements.remoteVideo.muted = true;
    (elements.remoteVideo as any).playsInline = true;
    elements.remoteVideo
      .play()
      .catch((e) => log("remote play() failed(pub)", e?.message || String(e)));
  };
  return publisher;
}

export async function setupPublisherTracks(stream: MediaStream) {
  if (!publisher) throw new Error("publisher PC not initialized");
  const audioTrack = stream.getAudioTracks()[0];
  if (audioTrack) publisher.addTrack(audioTrack, stream);

  const videoTrack = stream.getVideoTracks()[0];
  if (!videoTrack) return;
  try {
    try {
      (videoTrack as any).contentHint = "detail";
    } catch {
      log("contentHint not supported");
    }

    const vtx = publisher.addTransceiver("video", {
      direction: "sendonly",
      sendEncodings: [
        {
          rid: "f",
          scaleResolutionDownBy: 1.0,
          maxBitrate: 2_500_000,
          maxFramerate: 30,
          active: true,
        },
        {
          rid: "h",
          scaleResolutionDownBy: 2.0,
          maxBitrate: 900_000,
          maxFramerate: 30,
          active: true,
        },
        {
          rid: "q",
          scaleResolutionDownBy: 4.0,
          maxBitrate: 300_000,
          maxFramerate: 30,
          active: true,
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
    log("enabled simulcast encodings (f/h/q) with replaceTrack");
  } catch (e: any) {
    log(
      "simulcast setup failed, fallback to single stream:",
      e?.message || String(e),
    );
    publisher.addTrack(videoTrack, stream);
  }
}

export async function addPublisherCandidate(cand: RTCIceCandidateInit) {
  if (!publisher) return;
  await publisher.addIceCandidate(cand);
}
