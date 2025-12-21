import { els } from "@ui/dom";

export function getServerUrl(): string {
  return (els.serverUrl.value || "").trim();
}

export function getSessionId(): string {
  return (els.sessionId.value || "").trim();
}

export function getUserId(): string {
  return (els.userId.value || "").trim();
}

export function isTrickle(): boolean {
  return !!els.trickle.checked;
}

export function setupControls(opts: {
  onStart: () => unknown | Promise<unknown>;
  onJoin: () => unknown | Promise<unknown>;
  onHangup: () => unknown | Promise<unknown>;
}) {
  const { onStart, onJoin, onHangup } = opts;
  els.btnStart.onclick = () => void onStart();
  els.btnJoin.onclick = () => void onJoin();
  els.btnHangup.onclick = () => void onHangup();
}
