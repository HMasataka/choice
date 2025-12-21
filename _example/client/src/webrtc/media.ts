import { els } from "@ui/dom";
import { log } from "@app/logger";

let localStream: MediaStream | null = null;

export function getLocalStream(): MediaStream | null {
  return localStream;
}

export async function startLocalMedia(): Promise<MediaStream> {
  if (localStream) return localStream;
  localStream = await navigator.mediaDevices.getUserMedia({
    video: {
      width: { ideal: 1280 },
      height: { ideal: 720 },
      frameRate: { ideal: 30 },
    },
    audio: true,
  });
  els.localVideo.srcObject = localStream;
  try {
    // Ensure autoplay policies allow local preview
    (els.localVideo as any).playsInline = true;
    await els.localVideo.play();
  } catch (e: any) {
    log("local preview play() failed", e?.message || String(e));
  }
  return localStream;
}

export function stopLocalMedia() {
  if (localStream) {
    localStream.getTracks().forEach((t) => t.stop());
  }
  localStream = null;
  els.localVideo.srcObject = null;
}
