import { elements } from "@ui/dom";

export function log(...args: any[]) {
  const line = args
    .map((a) => (typeof a === "string" ? a : JSON.stringify(a)))
    .join(" ");
  elements.log.textContent += line + "\n";
  elements.log.scrollTop = elements.log.scrollHeight;
}
