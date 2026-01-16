/**
 * FrameCryptor handles encryption/decryption of RTP frames using Insertable Streams API.
 * This implements the WebRTC Encoded Transform API for E2EE.
 */

import type { E2EEAlgorithm, E2EEFrameMetadata } from '../utils/types';
import { Logger } from '../utils/logger';

const IV_LENGTH = 12; // 96 bits for AES-GCM
const AUTH_TAG_LENGTH = 16; // 128 bits for AES-GCM
const METADATA_LENGTH = 12; // participantId hash (4) + frameCounter (4) + keyIndex (4)
const UNENCRYPTED_BYTES = 10; // First 10 bytes of payload remain unencrypted for codec headers

/**
 * Frame cryptor for encrypting/decrypting media frames
 */
export class FrameCryptor {
  private algorithm: E2EEAlgorithm;
  private logger: Logger;
  private frameCounter = 0;
  private keyIndex = 0;

  constructor(algorithm: E2EEAlgorithm = 'AES-GCM') {
    this.algorithm = algorithm;
    this.logger = new Logger(undefined, 'FrameCryptor');
  }

  /**
   * Encrypt an encoded video/audio frame
   * @param encodedFrame - The RTCEncodedVideoFrame or RTCEncodedAudioFrame
   * @param key - The encryption key
   * @param participantId - The participant ID (for metadata)
   * @returns Encrypted frame data
   */
  public async encryptFrame(
    encodedFrame: RTCEncodedVideoFrame | RTCEncodedAudioFrame,
    key: CryptoKey,
    participantId: string
  ): Promise<ArrayBuffer> {
    const data = new Uint8Array(encodedFrame.data);

    // Skip encryption for very small frames or key frames (preserve codec headers)
    if (data.byteLength < UNENCRYPTED_BYTES) {
      return encodedFrame.data;
    }

    // Split frame: unencrypted header + encrypted payload
    const unencryptedHeader = data.slice(0, UNENCRYPTED_BYTES);
    const payload = data.slice(UNENCRYPTED_BYTES);

    // Generate IV (Initialization Vector)
    const iv = crypto.getRandomValues(new Uint8Array(IV_LENGTH));

    // Encrypt payload
    const encryptedPayload = await this.encrypt(payload, key, iv);

    // Build metadata
    const metadata = this.buildMetadata(participantId);

    // Construct final frame: unencrypted header + IV + metadata + encrypted payload
    const encryptedFrame = new Uint8Array(
      unencryptedHeader.byteLength + IV_LENGTH + METADATA_LENGTH + encryptedPayload.byteLength
    );

    let offset = 0;
    encryptedFrame.set(unencryptedHeader, offset);
    offset += unencryptedHeader.byteLength;
    encryptedFrame.set(iv, offset);
    offset += IV_LENGTH;
    encryptedFrame.set(metadata, offset);
    offset += METADATA_LENGTH;
    encryptedFrame.set(new Uint8Array(encryptedPayload), offset);

    this.frameCounter++;
    return encryptedFrame.buffer;
  }

  /**
   * Decrypt an encoded video/audio frame
   * @param encryptedData - The encrypted frame data
   * @param key - The decryption key
   * @returns Decrypted frame data and metadata
   */
  public async decryptFrame(
    encryptedData: ArrayBuffer,
    key: CryptoKey
  ): Promise<{ data: ArrayBuffer; metadata: E2EEFrameMetadata }> {
    const data = new Uint8Array(encryptedData);

    // Check minimum size
    const minSize = UNENCRYPTED_BYTES + IV_LENGTH + METADATA_LENGTH + AUTH_TAG_LENGTH;
    if (data.byteLength < minSize) {
      throw new Error('Encrypted frame too small');
    }

    // Parse frame structure
    let offset = 0;
    const unencryptedHeader = data.slice(offset, UNENCRYPTED_BYTES);
    offset += UNENCRYPTED_BYTES;
    const iv = data.slice(offset, offset + IV_LENGTH);
    offset += IV_LENGTH;
    const metadataBytes = data.slice(offset, offset + METADATA_LENGTH);
    offset += METADATA_LENGTH;
    const encryptedPayload = data.slice(offset);

    // Parse metadata
    const metadata = this.parseMetadata(metadataBytes);

    // Decrypt payload
    const decryptedPayload = await this.decrypt(encryptedPayload, key, iv);

    // Reconstruct frame: unencrypted header + decrypted payload
    const decryptedFrame = new Uint8Array(
      unencryptedHeader.byteLength + decryptedPayload.byteLength
    );
    decryptedFrame.set(unencryptedHeader, 0);
    decryptedFrame.set(new Uint8Array(decryptedPayload), unencryptedHeader.byteLength);

    return {
      data: decryptedFrame.buffer,
      metadata,
    };
  }

  /**
   * Encrypt data using WebCrypto API
   */
  private async encrypt(data: Uint8Array, key: CryptoKey, iv: Uint8Array): Promise<ArrayBuffer> {
    return crypto.subtle.encrypt(
      {
        name: this.algorithm,
        iv,
        ...(this.algorithm === 'AES-GCM' ? { tagLength: AUTH_TAG_LENGTH * 8 } : {}),
      },
      key,
      data
    );
  }

  /**
   * Decrypt data using WebCrypto API
   */
  private async decrypt(
    data: Uint8Array,
    key: CryptoKey,
    iv: Uint8Array
  ): Promise<ArrayBuffer> {
    return crypto.subtle.decrypt(
      {
        name: this.algorithm,
        iv,
        ...(this.algorithm === 'AES-GCM' ? { tagLength: AUTH_TAG_LENGTH * 8 } : {}),
      },
      key,
      data
    );
  }

  /**
   * Build metadata bytes
   */
  private buildMetadata(participantId: string): Uint8Array {
    const metadata = new Uint8Array(METADATA_LENGTH);
    const view = new DataView(metadata.buffer);

    // Simple hash of participantId (4 bytes)
    const hash = this.simpleHash(participantId);
    view.setUint32(0, hash, false);

    // Frame counter (4 bytes)
    view.setUint32(4, this.frameCounter, false);

    // Key index (4 bytes)
    view.setUint32(8, this.keyIndex, false);

    return metadata;
  }

  /**
   * Parse metadata bytes
   */
  private parseMetadata(metadata: Uint8Array): E2EEFrameMetadata {
    const view = new DataView(metadata.buffer, metadata.byteOffset, metadata.byteLength);

    const participantIdHash = view.getUint32(0, false);
    const frameCounter = view.getUint32(4, false);
    const keyIndex = view.getUint32(8, false);

    return {
      participantId: participantIdHash.toString(16), // Return hash as hex string
      frameCounter,
      keyIndex,
    };
  }

  /**
   * Simple hash function for participant ID
   */
  private simpleHash(str: string): number {
    let hash = 0;
    for (let i = 0; i < str.length; i++) {
      const char = str.charCodeAt(i);
      hash = (hash << 5) - hash + char;
      hash = hash & hash; // Convert to 32-bit integer
    }
    return hash >>> 0; // Convert to unsigned
  }

  /**
   * Set key index for key rotation
   */
  public setKeyIndex(index: number): void {
    this.keyIndex = index;
    this.logger.debug('Key index updated', { keyIndex: index });
  }

  /**
   * Reset frame counter
   */
  public resetFrameCounter(): void {
    this.frameCounter = 0;
    this.logger.debug('Frame counter reset');
  }

  /**
   * Get current frame counter
   */
  public getFrameCounter(): number {
    return this.frameCounter;
  }
}
