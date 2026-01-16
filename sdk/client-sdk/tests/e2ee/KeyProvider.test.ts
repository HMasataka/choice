/**
 * KeyProvider Tests
 */

import { describe, it, expect, beforeEach } from '@jest/globals';
import { DefaultKeyProvider } from '../../src/e2ee/KeyProvider';

describe('DefaultKeyProvider', () => {
  let keyProvider: DefaultKeyProvider;
  const participantId = 'test-participant-123';

  beforeEach(() => {
    keyProvider = new DefaultKeyProvider('AES-GCM');
  });

  describe('generateKey', () => {
    it('should generate a new key for a participant', async () => {
      const key = await keyProvider.generateKey(participantId);

      expect(key).toBeDefined();
      expect(key.type).toBe('secret');
      expect(key.algorithm).toMatchObject({
        name: 'AES-GCM',
        length: 256,
      });
    });

    it('should store the generated key', async () => {
      await keyProvider.generateKey(participantId);
      const retrievedKey = await keyProvider.getKey(participantId);

      expect(retrievedKey).not.toBeNull();
    });
  });

  describe('getKey', () => {
    it('should return null for non-existent key', async () => {
      const key = await keyProvider.getKey('non-existent');

      expect(key).toBeNull();
    });

    it('should return the stored key', async () => {
      const originalKey = await keyProvider.generateKey(participantId);
      const retrievedKey = await keyProvider.getKey(participantId);

      expect(retrievedKey).toBe(originalKey);
    });
  });

  describe('setKey', () => {
    it('should store a key', async () => {
      const key = await crypto.subtle.generateKey(
        { name: 'AES-GCM', length: 256 },
        true,
        ['encrypt', 'decrypt']
      );

      await keyProvider.setKey(participantId, key);
      const retrievedKey = await keyProvider.getKey(participantId);

      expect(retrievedKey).toBe(key);
    });
  });

  describe('removeKey', () => {
    it('should remove a stored key', async () => {
      await keyProvider.generateKey(participantId);
      await keyProvider.removeKey(participantId);
      const retrievedKey = await keyProvider.getKey(participantId);

      expect(retrievedKey).toBeNull();
    });
  });

  describe('ratchetKey', () => {
    it('should derive a new key from the current key', async () => {
      const originalKey = await keyProvider.generateKey(participantId);
      const ratchetedKey = await keyProvider.ratchetKey(participantId);

      expect(ratchetedKey).not.toBe(originalKey);
      expect(ratchetedKey.algorithm).toMatchObject({
        name: 'AES-GCM',
        length: 256,
      });
    });

    it('should update the stored key', async () => {
      await keyProvider.generateKey(participantId);
      const ratchetedKey = await keyProvider.ratchetKey(participantId);
      const storedKey = await keyProvider.getKey(participantId);

      expect(storedKey).toBe(ratchetedKey);
    });

    it('should throw if no key exists', async () => {
      await expect(keyProvider.ratchetKey('non-existent')).rejects.toThrow(
        'No key found for participant non-existent'
      );
    });
  });

  describe('exportKey and importKey', () => {
    it('should export and import a key', async () => {
      await keyProvider.generateKey(participantId);
      const exportedKey = await keyProvider.exportKey(participantId);

      expect(exportedKey).toBeInstanceOf(ArrayBuffer);
      expect(exportedKey.byteLength).toBe(32); // 256 bits

      // Import into a new provider
      const newKeyProvider = new DefaultKeyProvider('AES-GCM');
      await newKeyProvider.importKey(participantId, exportedKey);
      const importedKey = await newKeyProvider.getKey(participantId);

      expect(importedKey).not.toBeNull();
    });

    it('should throw when exporting non-existent key', async () => {
      await expect(keyProvider.exportKey('non-existent')).rejects.toThrow(
        'No key found for participant non-existent'
      );
    });
  });

  describe('clearAll', () => {
    it('should clear all stored keys', async () => {
      await keyProvider.generateKey('participant-1');
      await keyProvider.generateKey('participant-2');
      await keyProvider.generateKey('participant-3');

      expect(keyProvider.getKeyCount()).toBe(3);

      keyProvider.clearAll();

      expect(keyProvider.getKeyCount()).toBe(0);
      expect(await keyProvider.getKey('participant-1')).toBeNull();
    });
  });

  describe('getKeyCount', () => {
    it('should return the correct number of keys', async () => {
      expect(keyProvider.getKeyCount()).toBe(0);

      await keyProvider.generateKey('participant-1');
      expect(keyProvider.getKeyCount()).toBe(1);

      await keyProvider.generateKey('participant-2');
      expect(keyProvider.getKeyCount()).toBe(2);

      await keyProvider.removeKey('participant-1');
      expect(keyProvider.getKeyCount()).toBe(1);
    });
  });

  describe('algorithm support', () => {
    it('should work with AES-CTR algorithm', async () => {
      const ctrKeyProvider = new DefaultKeyProvider('AES-CTR');
      const key = await ctrKeyProvider.generateKey(participantId);

      expect(key.algorithm).toMatchObject({
        name: 'AES-CTR',
        length: 256,
      });
    });
  });
});
