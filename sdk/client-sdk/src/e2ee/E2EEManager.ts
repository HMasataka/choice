/**
 * E2EEManager manages end-to-end encryption for WebRTC peer connections
 * using the Insertable Streams API (WebRTC Encoded Transform)
 */

import type { E2EEConfig, E2EEKeyProvider, E2EEAlgorithm } from '../utils/types';
import { FrameCryptor } from './FrameCryptor';
import { DefaultKeyProvider } from './KeyProvider';
import { Logger } from '../utils/logger';

/**
 * E2EE Manager for handling encryption/decryption of media frames
 */
export class E2EEManager {
  private config: Required<E2EEConfig>;
  private keyProvider: E2EEKeyProvider;
  private encryptors: Map<string, FrameCryptor> = new Map(); // trackId -> encryptor
  private decryptors: Map<string, FrameCryptor> = new Map(); // trackId -> decryptor
  private keyCache: Map<string, CryptoKey> = new Map(); // participantId -> key
  private keyIndex: Map<string, number> = new Map(); // participantId -> key index
  private lastFrameCounters: Map<string, number> = new Map(); // participantHash -> last frame counter
  private logger: Logger;
  private localParticipantId: string | null = null;

  constructor(config: E2EEConfig) {
    this.config = {
      enabled: config.enabled,
      algorithm: 'AES-GCM',
      ratchetStrategy: config.ratchetStrategy || 'manual',
      keyProvider: config.keyProvider || new DefaultKeyProvider(),
    };
    this.keyProvider = this.config.keyProvider;
    this.logger = new Logger(undefined, 'E2EEManager');

    if (!this.isSupported()) {
      throw new Error('Insertable Streams API is not supported in this browser');
    }

    this.logger.info('E2EEManager initialized', {
      algorithm: this.config.algorithm,
      ratchetStrategy: this.config.ratchetStrategy,
    });
  }

  /**
   * Check if Insertable Streams API is supported
   */
  public static isSupported(): boolean {
    return (
      typeof RTCRtpSender !== 'undefined' &&
      typeof RTCRtpSender.prototype.createEncodedStreams === 'function'
    );
  }

  /**
   * Check if Insertable Streams API is supported (instance method)
   */
  public isSupported(): boolean {
    return E2EEManager.isSupported();
  }

  /**
   * Set local participant ID
   */
  public setLocalParticipantId(participantId: string): void {
    this.localParticipantId = participantId;
    this.logger.debug('Local participant ID set', { participantId });
  }

  /**
   * Setup encryption for an RTCRtpSender (outgoing track)
   */
  public async setupSenderTransform(
    sender: RTCRtpSender,
    trackId: string,
    participantId: string
  ): Promise<void> {
    if (!this.config.enabled) {
      return;
    }

    // Create frame cryptor
    const cryptor = new FrameCryptor(this.config.algorithm);
    this.encryptors.set(trackId, cryptor);

    // Create encoded streams
    const streams = sender.createEncodedStreams();
    const { readable, writable } = streams;

    // Setup transform stream
    const transformStream = new TransformStream({
      transform: async (encodedFrame, controller) => {
        try {
          let key = this.keyCache.get(participantId);
          if (!key) {
            key = await this.keyProvider.getKey(participantId);
            if (key) {
              this.keyCache.set(participantId, key);
            }
          }
          if (!key) {
            this.logger.error('No encryption key available for participant', { participantId });
            return;
          }

          const index = this.keyIndex.get(participantId) ?? 0;
          cryptor.setKeyIndex(index);

          const encryptedData = await cryptor.encryptFrame(encodedFrame, key, participantId);
          encodedFrame.data = encryptedData;
          controller.enqueue(encodedFrame);
        } catch (error) {
          this.logger.error('Failed to encrypt frame', { trackId, error });
        }
      },
    });

    // Pipe streams
    readable.pipeThrough(transformStream).pipeTo(writable).catch((error) => {
      this.logger.error('Sender transform pipeline error', { trackId, error });
    });

    this.logger.info('Sender transform setup complete', { trackId, participantId });
  }

  /**
   * Setup decryption for an RTCRtpReceiver (incoming track)
   */
  public async setupReceiverTransform(
    receiver: RTCRtpReceiver,
    trackId: string,
    participantId: string
  ): Promise<void> {
    if (!this.config.enabled) {
      return;
    }

    // Create frame cryptor
    const cryptor = new FrameCryptor(this.config.algorithm);
    this.decryptors.set(trackId, cryptor);

    // Create encoded streams
    const streams = receiver.createEncodedStreams();
    const { readable, writable } = streams;

    // Setup transform stream
    const transformStream = new TransformStream({
      transform: async (encodedFrame, controller) => {
        try {
          let key = this.keyCache.get(participantId);
          if (!key) {
            key = await this.keyProvider.getKey(participantId);
            if (key) {
              this.keyCache.set(participantId, key);
            }
          }
          if (!key) {
            this.logger.error('No decryption key available for participant', { participantId });
            return;
          }

          const { data, metadata } = await cryptor.decryptFrame(encodedFrame.data, key);
          const lastCounter = this.lastFrameCounters.get(metadata.participantId);
          if (lastCounter !== undefined && metadata.frameCounter <= lastCounter) {
            this.logger.warn('Replay detected, dropping frame', {
              trackId,
              participantId: metadata.participantId,
              frameCounter: metadata.frameCounter,
              lastCounter,
            });
            return;
          }
          this.lastFrameCounters.set(metadata.participantId, metadata.frameCounter);

          encodedFrame.data = data;
          controller.enqueue(encodedFrame);

          // Log frame metadata for debugging
          this.logger.debug('Frame decrypted', {
            trackId,
            frameCounter: metadata.frameCounter,
            keyIndex: metadata.keyIndex,
          });
        } catch (error) {
          this.logger.error('Failed to decrypt frame', { trackId, error });
          // Drop encrypted frame on error (don't forward unencrypted)
          // controller.enqueue(encodedFrame); // Uncomment to forward on error
        }
      },
    });

    // Pipe streams
    readable.pipeThrough(transformStream).pipeTo(writable).catch((error) => {
      this.logger.error('Receiver transform pipeline error', { trackId, error });
    });

    this.logger.info('Receiver transform setup complete', { trackId, participantId });
  }

  /**
   * Remove transform for a track
   */
  public removeTransform(trackId: string): void {
    this.encryptors.delete(trackId);
    this.decryptors.delete(trackId);
    this.logger.debug('Transform removed', { trackId });
  }

  /**
   * Set encryption key for a participant
   */
  public async setKey(participantId: string, key: CryptoKey): Promise<void> {
    await this.keyProvider.setKey(participantId, key);
    this.keyCache.set(participantId, key);
    if (!this.keyIndex.has(participantId)) {
      this.keyIndex.set(participantId, 0);
    }
    this.logger.info('Encryption key set', { participantId });
  }

  /**
   * Remove encryption key for a participant
   */
  public async removeKey(participantId: string): Promise<void> {
    await this.keyProvider.removeKey(participantId);
    this.keyCache.delete(participantId);
    this.keyIndex.delete(participantId);
    this.logger.info('Encryption key removed', { participantId });
  }

  /**
   * Ratchet key for a participant
   */
  public async ratchetKey(participantId: string): Promise<CryptoKey> {
    const newKey = await this.keyProvider.ratchetKey(participantId);
    this.keyCache.set(participantId, newKey);
    const nextIndex = (this.keyIndex.get(participantId) ?? 0) + 1;
    this.keyIndex.set(participantId, nextIndex);
    this.logger.info('Key ratcheted', { participantId });
    return newKey;
  }

  /**
   * Enable E2EE
   */
  public enable(): void {
    this.config.enabled = true;
    this.logger.info('E2EE enabled');
  }

  /**
   * Disable E2EE
   */
  public disable(): void {
    this.config.enabled = false;
    this.encryptors.clear();
    this.decryptors.clear();
    this.lastFrameCounters.clear();
    this.logger.info('E2EE disabled');
  }

  /**
   * Check if E2EE is enabled
   */
  public isEnabled(): boolean {
    return this.config.enabled;
  }

  /**
   * Get encryption algorithm
   */
  public getAlgorithm(): E2EEAlgorithm {
    return this.config.algorithm;
  }

  /**
   * Get key provider
   */
  public getKeyProvider(): E2EEKeyProvider {
    return this.keyProvider;
  }
}
