/**
 * KeyProvider manages encryption keys for E2EE
 */

import type { E2EEKeyProvider, E2EEAlgorithm } from '../utils/types';
import { Logger } from '../utils/logger';

/**
 * Default key provider implementation using Web Crypto API
 */
export class DefaultKeyProvider implements E2EEKeyProvider {
  private keys: Map<string, CryptoKey> = new Map();
  private algorithm: E2EEAlgorithm;
  private logger: Logger;

  constructor(algorithm: E2EEAlgorithm = 'AES-GCM') {
    this.algorithm = algorithm;
    this.logger = new Logger(undefined, 'KeyProvider');
  }

  /**
   * Get encryption key for a participant
   */
  public async getKey(participantId: string): Promise<CryptoKey | null> {
    const key = this.keys.get(participantId);
    if (key === undefined) {
      this.logger.warn('Key not found for participant', { participantId });
      return null;
    }
    return key;
  }

  /**
   * Set encryption key for a participant
   */
  public async setKey(participantId: string, key: CryptoKey): Promise<void> {
    this.keys.set(participantId, key);
    this.logger.debug('Key set for participant', { participantId });
  }

  /**
   * Remove encryption key for a participant
   */
  public async removeKey(participantId: string): Promise<void> {
    this.keys.delete(participantId);
    this.logger.debug('Key removed for participant', { participantId });
  }

  /**
   * Ratchet key (derive new key from current key)
   * Uses HKDF (HMAC-based Key Derivation Function) for key derivation
   */
  public async ratchetKey(participantId: string): Promise<CryptoKey> {
    const currentKey = await this.getKey(participantId);
    if (currentKey === null) {
      throw new Error(`No key found for participant ${participantId}`);
    }

    // Export current key
    const keyData = await crypto.subtle.exportKey('raw', currentKey);

    // Derive new key using HKDF
    const salt = crypto.getRandomValues(new Uint8Array(16));
    const info = new TextEncoder().encode(`ratchet-${participantId}`);

    // Import key material for HKDF
    const hkdfKey = await crypto.subtle.importKey(
      'raw',
      keyData,
      'HKDF',
      false,
      ['deriveKey']
    );

    // Derive new key
    const newKey = await crypto.subtle.deriveKey(
      {
        name: 'HKDF',
        hash: 'SHA-256',
        salt,
        info,
      },
      hkdfKey,
      {
        name: this.algorithm,
        length: 256,
      },
      true,
      ['encrypt', 'decrypt']
    );

    // Store new key
    await this.setKey(participantId, newKey);

    this.logger.info('Key ratcheted for participant', { participantId });
    return newKey;
  }

  /**
   * Generate a new random key for a participant
   */
  public async generateKey(participantId: string): Promise<CryptoKey> {
    const key = await crypto.subtle.generateKey(
      {
        name: this.algorithm,
        length: 256,
      },
      true,
      ['encrypt', 'decrypt']
    );

    await this.setKey(participantId, key);
    this.logger.info('New key generated for participant', { participantId });
    return key;
  }

  /**
   * Import a key from raw bytes
   */
  public async importKey(participantId: string, keyData: ArrayBuffer): Promise<CryptoKey> {
    const key = await crypto.subtle.importKey(
      'raw',
      keyData,
      {
        name: this.algorithm,
        length: 256,
      },
      true,
      ['encrypt', 'decrypt']
    );

    await this.setKey(participantId, key);
    this.logger.info('Key imported for participant', { participantId });
    return key;
  }

  /**
   * Export a key to raw bytes
   */
  public async exportKey(participantId: string): Promise<ArrayBuffer> {
    const key = await this.getKey(participantId);
    if (key === null) {
      throw new Error(`No key found for participant ${participantId}`);
    }

    return crypto.subtle.exportKey('raw', key);
  }

  /**
   * Clear all keys
   */
  public clearAll(): void {
    this.keys.clear();
    this.logger.info('All keys cleared');
  }

  /**
   * Get number of stored keys
   */
  public getKeyCount(): number {
    return this.keys.size;
  }
}
