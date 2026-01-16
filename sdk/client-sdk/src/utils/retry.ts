/**
 * Retry utility with exponential backoff
 */

import type { ReconnectConfig } from './types';

/** Default reconnect configuration */
export const DEFAULT_RECONNECT_CONFIG: ReconnectConfig = {
  maxAttempts: 5,
  initialDelay: 1000,
  maxDelay: 30000,
  factor: 2,
};

/**
 * Calculate delay for a given attempt with exponential backoff
 */
export function calculateBackoffDelay(
  attempt: number,
  config: ReconnectConfig
): number {
  const delay = config.initialDelay * Math.pow(config.factor, attempt);
  // Add jitter (0-10% of delay)
  const jitter = delay * Math.random() * 0.1;
  return Math.min(delay + jitter, config.maxDelay);
}

/**
 * Sleep for a specified duration
 */
export function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Retry a function with exponential backoff
 */
export async function retry<T>(
  fn: () => Promise<T>,
  config: Partial<ReconnectConfig> = {},
  onRetry?: (attempt: number, error: Error, nextDelay: number) => void
): Promise<T> {
  const fullConfig = { ...DEFAULT_RECONNECT_CONFIG, ...config };
  let lastError: Error | undefined;

  for (let attempt = 0; attempt < fullConfig.maxAttempts; attempt++) {
    try {
      return await fn();
    } catch (error) {
      lastError = error instanceof Error ? error : new Error(String(error));

      if (attempt < fullConfig.maxAttempts - 1) {
        const delay = calculateBackoffDelay(attempt, fullConfig);
        onRetry?.(attempt + 1, lastError, delay);
        await sleep(delay);
      }
    }
  }

  throw lastError ?? new Error('Retry failed');
}

/**
 * Create a retryable function
 */
export function createRetryable<T>(
  fn: () => Promise<T>,
  config: Partial<ReconnectConfig> = {}
): () => Promise<T> {
  return () => retry(fn, config);
}
