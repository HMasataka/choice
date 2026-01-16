/**
 * JSON-RPC 2.0 client over WebSocket
 */

import { EventEmitter } from '../events/EventEmitter';
import { SFUError, ErrorCodes } from '../errors/SFUError';
import { Logger } from '../utils/logger';
import type { LoggerConfig } from '../utils/types';
import type {
  JsonRpcRequest,
  JsonRpcResponse,
  JsonRpcNotification,
  JsonRpcMessage,
} from './types';

/** Pending request */
interface PendingRequest<T = unknown> {
  resolve: (result: T) => void;
  reject: (error: Error) => void;
  timeout: ReturnType<typeof setTimeout>;
}

/** JSON-RPC client events */
interface JsonRpcClientEvents {
  notification: JsonRpcNotification;
  open: void;
  close: CloseEvent;
  error: Event;
}

/** JSON-RPC client options */
export interface JsonRpcClientOptions {
  requestTimeout?: number;
  logger?: LoggerConfig;
}

/**
 * JSON-RPC 2.0 client
 */
export class JsonRpcClient extends EventEmitter<JsonRpcClientEvents> {
  private ws: WebSocket | null = null;
  private pendingRequests: Map<string, PendingRequest> = new Map();
  private requestId = 0;
  private readonly requestTimeout: number;
  private readonly logger: Logger;

  constructor(options: JsonRpcClientOptions = {}) {
    super();
    this.requestTimeout = options.requestTimeout ?? 30000;
    this.logger = new Logger(options.logger, 'JsonRpc');
  }

  /**
   * Connect to WebSocket server
   */
  public connect(url: string): Promise<void> {
    return new Promise((resolve, reject) => {
      if (this.ws !== null) {
        this.ws.close();
      }

      this.logger.debug(`Connecting to ${url}`);
      this.ws = new WebSocket(url);

      const onOpen = (): void => {
        this.logger.info('WebSocket connected');
        cleanup();
        this.emit('open', undefined);
        resolve();
      };

      const onError = (event: Event): void => {
        this.logger.error('WebSocket connection error', event);
        cleanup();
        reject(new SFUError(ErrorCodes.CONNECTION_FAILED, 'WebSocket connection failed'));
      };

      const onClose = (event: CloseEvent): void => {
        this.logger.warn(`WebSocket closed: code=${event.code}, reason=${event.reason}`);
        cleanup();
        reject(new SFUError(ErrorCodes.CONNECTION_FAILED, `WebSocket closed: ${event.reason}`));
      };

      const cleanup = (): void => {
        this.ws?.removeEventListener('open', onOpen);
        this.ws?.removeEventListener('error', onError);
        this.ws?.removeEventListener('close', onClose);
      };

      this.ws.addEventListener('open', onOpen);
      this.ws.addEventListener('error', onError);
      this.ws.addEventListener('close', onClose);

      // Set up permanent handlers after connection
      this.ws.addEventListener('message', this.handleMessage.bind(this));
      this.ws.addEventListener('close', this.handleClose.bind(this));
      this.ws.addEventListener('error', this.handleError.bind(this));
    });
  }

  /**
   * Disconnect from WebSocket server
   */
  public disconnect(): void {
    if (this.ws !== null) {
      this.logger.info('Disconnecting WebSocket');
      this.ws.close(1000, 'Client disconnect');
      this.ws = null;
    }

    // Reject all pending requests
    for (const [id, request] of this.pendingRequests) {
      clearTimeout(request.timeout);
      request.reject(new SFUError(ErrorCodes.CONNECTION_FAILED, 'Connection closed'));
      this.pendingRequests.delete(id);
    }
  }

  /**
   * Check if connected
   */
  public isConnected(): boolean {
    return this.ws !== null && this.ws.readyState === WebSocket.OPEN;
  }

  /**
   * Send a JSON-RPC request and wait for response
   */
  public async request<TParams, TResult>(
    method: string,
    params?: TParams
  ): Promise<TResult> {
    if (!this.isConnected()) {
      throw new SFUError(ErrorCodes.CONNECTION_FAILED, 'Not connected');
    }

    const id = this.generateId();
    const request: JsonRpcRequest<TParams> = {
      jsonrpc: '2.0',
      id,
      method,
      params,
    };

    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        this.pendingRequests.delete(id);
        reject(new SFUError(ErrorCodes.TIMEOUT, `Request timeout: ${method}`));
      }, this.requestTimeout);

      this.pendingRequests.set(id, {
        resolve: resolve as (result: unknown) => void,
        reject,
        timeout,
      });

      this.send(request);
      this.logger.debug(`Request sent: ${method}`, { id, params });
    });
  }

  /**
   * Send a JSON-RPC notification (no response expected)
   */
  public notify<TParams>(method: string, params?: TParams): void {
    if (!this.isConnected()) {
      throw new SFUError(ErrorCodes.CONNECTION_FAILED, 'Not connected');
    }

    const notification: JsonRpcNotification<TParams> = {
      jsonrpc: '2.0',
      method,
      params,
    };

    this.send(notification);
    this.logger.debug(`Notification sent: ${method}`, { params });
  }

  /**
   * Send a message over WebSocket
   */
  private send(message: JsonRpcMessage): void {
    if (this.ws === null || this.ws.readyState !== WebSocket.OPEN) {
      throw new SFUError(ErrorCodes.CONNECTION_FAILED, 'WebSocket not connected');
    }

    this.ws.send(JSON.stringify(message));
  }

  /**
   * Handle incoming WebSocket message
   */
  private handleMessage(event: MessageEvent): void {
    let message: JsonRpcMessage;

    try {
      message = JSON.parse(event.data as string) as JsonRpcMessage;
    } catch {
      this.logger.error('Failed to parse message', event.data);
      return;
    }

    // Check if it's a response (has id and result/error)
    if ('id' in message && message.id !== undefined) {
      this.handleResponse(message as JsonRpcResponse);
    } else if ('method' in message) {
      // It's a notification
      this.logger.debug(`Notification received: ${message.method}`, message.params);
      this.emit('notification', message as JsonRpcNotification);
    }
  }

  /**
   * Handle JSON-RPC response
   */
  private handleResponse(response: JsonRpcResponse): void {
    const pending = this.pendingRequests.get(response.id);
    if (pending === undefined) {
      this.logger.warn(`Received response for unknown request: ${response.id}`);
      return;
    }

    clearTimeout(pending.timeout);
    this.pendingRequests.delete(response.id);

    if (response.error !== undefined) {
      this.logger.debug(`Response error: ${response.id}`, response.error);
      pending.reject(SFUError.fromJsonRpcError(response.error));
    } else {
      this.logger.debug(`Response success: ${response.id}`, response.result);
      pending.resolve(response.result);
    }
  }

  /**
   * Handle WebSocket close
   */
  private handleClose(event: CloseEvent): void {
    this.logger.info(`WebSocket closed: code=${event.code}, reason=${event.reason}`);

    // Reject all pending requests
    for (const [id, request] of this.pendingRequests) {
      clearTimeout(request.timeout);
      request.reject(new SFUError(ErrorCodes.CONNECTION_FAILED, 'Connection closed'));
      this.pendingRequests.delete(id);
    }

    this.emit('close', event);
  }

  /**
   * Handle WebSocket error
   */
  private handleError(event: Event): void {
    this.logger.error('WebSocket error', event);
    this.emit('error', event);
  }

  /**
   * Generate unique request ID
   */
  private generateId(): string {
    return `${Date.now()}-${++this.requestId}`;
  }
}
