/**
 * useSFUClient hook - manages SFU client connection
 */

import { useState, useEffect, useCallback, useRef } from 'react';
import type { SFUClient, SFUClientConfig, ConnectionState } from '@sfu/client-sdk';

/** useSFUClient options */
export interface UseSFUClientOptions extends SFUClientConfig {
  /** Auto-connect on mount */
  autoConnect?: boolean;
}

/** useSFUClient return type */
export interface UseSFUClientReturn {
  /** SFU client instance */
  client: SFUClient | null;
  /** Connection state */
  connectionState: ConnectionState;
  /** Connect to server */
  connect: () => Promise<void>;
  /** Disconnect from server */
  disconnect: () => void;
  /** Error if any */
  error: Error | null;
}

/**
 * Hook for managing SFU client connection
 */
export function useSFUClient(options: UseSFUClientOptions): UseSFUClientReturn {
  const [client, setClient] = useState<SFUClient | null>(null);
  const [connectionState, setConnectionState] =
    useState<ConnectionState>('disconnected');
  const [error, setError] = useState<Error | null>(null);
  const clientRef = useRef<SFUClient | null>(null);
  const mountedRef = useRef(true);

  // Initialize client
  useEffect(() => {
    mountedRef.current = true;

    // Dynamic import to avoid SSR issues
    void import('@sfu/client-sdk').then(({ SFUClient: SFUClientClass }) => {
      if (!mountedRef.current) return;

      const newClient = new SFUClientClass({
        url: options.url,
        autoReconnect: options.autoReconnect,
        reconnect: options.reconnect,
        logger: options.logger,
        iceServers: options.iceServers,
      });

      // Set up event handlers
      newClient.on('connecting', () => {
        if (mountedRef.current) {
          setConnectionState('connecting');
        }
      });

      newClient.on('connected', () => {
        if (mountedRef.current) {
          setConnectionState('connected');
          setError(null);
        }
      });

      newClient.on('disconnected', () => {
        if (mountedRef.current) {
          setConnectionState('disconnected');
        }
      });

      newClient.on('reconnecting', () => {
        if (mountedRef.current) {
          setConnectionState('reconnecting');
        }
      });

      newClient.on('reconnected', () => {
        if (mountedRef.current) {
          setConnectionState('connected');
        }
      });

      newClient.on('error', (err) => {
        if (mountedRef.current) {
          setError(err);
        }
      });

      clientRef.current = newClient;
      setClient(newClient);

      // Auto-connect if enabled
      if (options.autoConnect === true) {
        void newClient.connect().catch((err: unknown) => {
          if (mountedRef.current && err instanceof Error) {
            setError(err);
          }
        });
      }
    });

    return () => {
      mountedRef.current = false;
      if (clientRef.current !== null) {
        clientRef.current.disconnect();
        clientRef.current = null;
      }
    };
  }, [options.url]); // Only recreate on URL change

  const connect = useCallback(async () => {
    if (clientRef.current === null) {
      throw new Error('Client not initialized');
    }
    setError(null);
    await clientRef.current.connect();
  }, []);

  const disconnect = useCallback(() => {
    if (clientRef.current !== null) {
      clientRef.current.disconnect();
    }
  }, []);

  return {
    client,
    connectionState,
    connect,
    disconnect,
    error,
  };
}
