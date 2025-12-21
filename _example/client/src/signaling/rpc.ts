// JSON-RPC 2.0 helpers and URL resolver
// Pure utilities to build/inspect JSON-RPC messages and resolve WS URLs.

export type JsonRpcVersion = "2.0";

export interface JsonRpcRequest<P = unknown> {
  jsonrpc: JsonRpcVersion;
  method: string;
  params?: P;
  id: number;
}

export interface JsonRpcNotification<P = unknown> {
  jsonrpc: JsonRpcVersion;
  method: string;
  params?: P;
}

export interface JsonRpcSuccess<R = unknown> {
  jsonrpc: JsonRpcVersion;
  id: number;
  result: R;
}

export interface JsonRpcError {
  jsonrpc: JsonRpcVersion;
  id: number;
  error: unknown;
}

export function makeRequest<P = unknown>(
  id: number,
  method: string,
  params?: P,
): JsonRpcRequest<P> {
  return { jsonrpc: "2.0", id, method, params };
}

export function makeNotification<P = unknown>(
  method: string,
  params?: P,
): JsonRpcNotification<P> {
  return { jsonrpc: "2.0", method, params };
}

export function isResponse(msg: any): msg is JsonRpcSuccess | JsonRpcError {
  return msg && typeof msg === "object" && "id" in msg;
}

export function resolveWsUrl(path: string): string {
  try {
    if (path.startsWith("ws://") || path.startsWith("wss://")) return path;
    if (path.startsWith("http://") || path.startsWith("https://")) {
      const u = new URL(path);
      u.protocol = u.protocol.replace("http", "ws");
      return u.toString();
    }
    const origin = new URL(window.location.origin);
    origin.protocol = origin.protocol.replace("http", "ws");
    return origin.origin + path;
  } catch {
    return path;
  }
}
