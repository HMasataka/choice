// VSCode-style JSON-RPC framing codec (Content-Length headers)
// Pure encode/decode helpers for testability.

export function encodeVS(obj: any): string {
  const json = JSON.stringify(obj);
  const bytes = new TextEncoder().encode(json).length;
  return `Content-Length: ${bytes}\r\n\r\n${json}`;
}

export function decodeVS(text: string): {
  messages: string[];
  remaining: string;
} {
  let buf = text;
  const messages: string[] = [];
  for (;;) {
    const sep = buf.indexOf("\r\n\r\n");
    if (sep === -1) break;
    const header = buf.slice(0, sep);
    const m = /Content-Length:\s*(\d+)/i.exec(header);
    if (!m) break;
    const len = parseInt(m[1], 10);
    const start = sep + 4;
    const end = start + len;
    if (buf.length < end) break; // incomplete frame
    messages.push(buf.slice(start, end));
    buf = buf.slice(end);
    if (!buf) break;
  }
  return { messages, remaining: buf };
}
