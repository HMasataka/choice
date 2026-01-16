/**
 * Logger utility for SDK
 */

import type { LoggerConfig } from './types';

type LogLevel = 'error' | 'warn' | 'info' | 'debug';

const LOG_LEVELS: Record<LogLevel, number> = {
  error: 0,
  warn: 1,
  info: 2,
  debug: 3,
};

/**
 * SDK Logger
 */
export class Logger {
  private level: LogLevel;
  private handler?: (level: string, message: string, data?: unknown) => void;
  private prefix: string;

  constructor(config?: LoggerConfig, prefix = 'SFU') {
    this.level = config?.level ?? 'info';
    this.handler = config?.handler;
    this.prefix = prefix;
  }

  private shouldLog(level: LogLevel): boolean {
    return LOG_LEVELS[level] <= LOG_LEVELS[this.level];
  }

  private log(level: LogLevel, message: string, data?: unknown): void {
    if (!this.shouldLog(level)) {
      return;
    }

    const formattedMessage = `[${this.prefix}] ${message}`;

    if (this.handler !== undefined) {
      this.handler(level, formattedMessage, data);
      return;
    }

    const timestamp = new Date().toISOString();
    const logMessage = `${timestamp} [${level.toUpperCase()}] ${formattedMessage}`;

    switch (level) {
      case 'error':
        if (data !== undefined) {
          console.error(logMessage, data);
        } else {
          console.error(logMessage);
        }
        break;
      case 'warn':
        if (data !== undefined) {
          console.warn(logMessage, data);
        } else {
          console.warn(logMessage);
        }
        break;
      case 'info':
      case 'debug':
        // Using console.warn to avoid eslint no-console rule
        // In production, these would be handled by the custom handler
        if (this.handler === undefined) {
          // eslint-disable-next-line no-console
          console.log(logMessage, data ?? '');
        }
        break;
    }
  }

  public error(message: string, data?: unknown): void {
    this.log('error', message, data);
  }

  public warn(message: string, data?: unknown): void {
    this.log('warn', message, data);
  }

  public info(message: string, data?: unknown): void {
    this.log('info', message, data);
  }

  public debug(message: string, data?: unknown): void {
    this.log('debug', message, data);
  }

  /**
   * Create a child logger with a specific prefix
   */
  public child(prefix: string): Logger {
    return new Logger(
      { level: this.level, handler: this.handler },
      `${this.prefix}:${prefix}`
    );
  }

  /**
   * Set log level
   */
  public setLevel(level: LogLevel): void {
    this.level = level;
  }
}
