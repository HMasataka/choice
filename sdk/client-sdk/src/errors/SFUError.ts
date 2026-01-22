/**
 * SFU Error class and error codes
 */

/** Error codes */
export const ErrorCodes = {
  // JSON-RPC standard errors
  PARSE_ERROR: -32700,
  INVALID_REQUEST: -32600,
  METHOD_NOT_FOUND: -32601,
  INVALID_PARAMS: -32602,
  INTERNAL_ERROR: -32603,

  // Application errors
  ROOM_NOT_FOUND: 1001,
  ROOM_FULL: 1002,
  UNAUTHORIZED: 1003,
  ALREADY_JOINED: 1004,
  NOT_IN_ROOM: 1005,
  TRACK_NOT_FOUND: 1006,
  INVALID_SDP: 1007,
  ICE_FAILURE: 1008,
  SESSION_EXPIRED: 1009,

  // Client-side errors
  CONNECTION_FAILED: 2001,
  SIGNALING_ERROR: 2002,
  MEDIA_ERROR: 2003,
  TIMEOUT: 2004,
} as const;

export type ErrorCode = (typeof ErrorCodes)[keyof typeof ErrorCodes];

/**
 * SFU Error class
 */
export class SFUError extends Error {
  public readonly code: number;
  public readonly data?: unknown;

  constructor(code: number, message: string, data?: unknown) {
    super(message);
    this.name = 'SFUError';
    this.code = code;
    this.data = data;

    // Maintain proper stack trace for where error was thrown
    if (Error.captureStackTrace !== undefined) {
      Error.captureStackTrace(this, SFUError);
    }
  }

  /**
   * Check if error is a specific error code
   */
  public is(code: number): boolean {
    return this.code === code;
  }

  /**
   * Check if error is recoverable (can retry)
   */
  public isRecoverable(): boolean {
    const recoverableCodes: number[] = [
      ErrorCodes.CONNECTION_FAILED,
      ErrorCodes.SIGNALING_ERROR,
      ErrorCodes.TIMEOUT,
      ErrorCodes.ICE_FAILURE,
    ];
    return recoverableCodes.includes(this.code);
  }

  /**
   * Create error from JSON-RPC error response
   */
  public static fromJsonRpcError(error: {
    code: number;
    message: string;
    data?: unknown;
  }): SFUError {
    return new SFUError(error.code, error.message, error.data);
  }
}
