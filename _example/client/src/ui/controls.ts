import { elements } from "@ui/dom";

export function getServerUrl(): string {
  return (elements.serverURL.value || "").trim();
}

export function getSessionId(): string {
  return (elements.sessionID.value || "").trim();
}

export function getUserId(): string {
  return (elements.userID.value || "").trim();
}

export function isTrickle(): boolean {
  return !!elements.trickle.checked;
}

export function setupControls(opts: {
  onStart: () => unknown | Promise<unknown>;
  onJoin: () => unknown | Promise<unknown>;
  onHangup: () => unknown | Promise<unknown>;
}) {
  const { onStart, onJoin, onHangup } = opts;
  elements.buttonStart.onclick = () => void onStart();
  elements.buttonJoin.onclick = () => void onJoin();
  elements.buttonHangup.onclick = () => void onHangup();
}
