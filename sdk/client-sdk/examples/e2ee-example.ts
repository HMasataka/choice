/**
 * E2EE (End-to-End Encryption) Usage Example
 *
 * This example demonstrates how to use E2EE (AES-GCM only) with the SFU Client SDK.
 * E2EE ensures that media is encrypted on the sender side and decrypted on the receiver side,
 * with the server only forwarding encrypted frames without being able to decrypt them.
 */

import {
  SFUClient,
  E2EEManager,
  DefaultKeyProvider,
  type E2EEKeyProvider,
} from '../src';

/**
 * Example 1: Basic E2EE setup with default key provider
 */
async function basicE2EEExample() {
  // Create SFU client with E2EE enabled
  const client = new SFUClient({
    url: 'wss://sfu.example.com/ws',
    e2ee: {
      enabled: true,
      ratchetStrategy: 'manual',
    },
  });

  // Join room
  const token = 'your-jwt-token';
  const room = await client.join(token);

  // Generate encryption key for local participant
  const keyProvider = new DefaultKeyProvider();
  const localKey = await keyProvider.generateKey(room.localParticipant.id);

  // Create E2EE manager
  const e2eeManager = new E2EEManager({
    enabled: true,
    keyProvider,
  });

  e2eeManager.setLocalParticipantId(room.localParticipant.id);

  // Publish media with E2EE
  const stream = await navigator.mediaDevices.getUserMedia({
    video: true,
    audio: true,
  });

  for (const track of stream.getTracks()) {
    const localTrack = await room.localParticipant.publish(track, {
      name: track.kind,
      simulcast: track.kind === 'video',
    });

    // Setup encryption for outgoing track
    const sender = (room as any).pc?.getSender(track.id);
    if (sender) {
      await e2eeManager.setupSenderTransform(
        sender,
        localTrack.id,
        room.localParticipant.id
      );
    }
  }

  // Handle incoming participants and setup decryption
  room.on('participantJoined', async (participant) => {
    console.log(`Participant joined: ${participant.id}`);

    // Exchange keys with the new participant (using your key exchange mechanism)
    // For this example, we generate a shared key
    const sharedKey = await keyProvider.generateKey(participant.id);

    // Setup decryption for incoming tracks
    room.on('trackPublished', async (track, pub) => {
      if (pub.id === participant.id) {
        const remoteTrack = await participant.subscribe(track.id);

        // Get the receiver and setup decryption
        const receiver = (remoteTrack as any).receiver;
        if (receiver) {
          await e2eeManager.setupReceiverTransform(
            receiver,
            track.id,
            participant.id
          );
        }
      }
    });
  });

  console.log('E2EE enabled and ready');
}

/**
 * Example 2: Custom key provider with external key management
 */
class CustomKeyProvider implements E2EEKeyProvider {
  private keys = new Map<string, CryptoKey>();

  async getKey(participantId: string): Promise<CryptoKey | null> {
    // Fetch key from your key management service
    const key = this.keys.get(participantId);
    return key || null;
  }

  async setKey(participantId: string, key: CryptoKey): Promise<void> {
    this.keys.set(participantId, key);
  }

  async removeKey(participantId: string): Promise<void> {
    this.keys.delete(participantId);
  }

  async ratchetKey(participantId: string): Promise<CryptoKey> {
    // Implement your key ratcheting logic
    // This example uses HKDF for demonstration
    const currentKey = await this.getKey(participantId);
    if (!currentKey) {
      throw new Error('No key found');
    }

    const keyData = await crypto.subtle.exportKey('raw', currentKey);
    const salt = crypto.getRandomValues(new Uint8Array(16));
    const info = new TextEncoder().encode(`ratchet-${participantId}`);

    const hkdfKey = await crypto.subtle.importKey(
      'raw',
      keyData,
      'HKDF',
      false,
      ['deriveKey']
    );

    const newKey = await crypto.subtle.deriveKey(
      {
        name: 'HKDF',
        hash: 'SHA-256',
        salt,
        info,
      },
      hkdfKey,
      {
        name: 'AES-GCM',
        length: 256,
      },
      true,
      ['encrypt', 'decrypt']
    );

    await this.setKey(participantId, newKey);
    return newKey;
  }
}

async function customKeyProviderExample() {
  const customKeyProvider = new CustomKeyProvider();

  const client = new SFUClient({
    url: 'wss://sfu.example.com/ws',
    e2ee: {
      enabled: true,
      keyProvider: customKeyProvider,
    },
  });

  console.log('Using custom key provider');
}

/**
 * Example 3: Key rotation
 */
async function keyRotationExample() {
  const keyProvider = new DefaultKeyProvider();
  const e2eeManager = new E2EEManager({
    enabled: true,
    keyProvider,
  });

  const participantId = 'participant-123';

  // Generate initial key
  await keyProvider.generateKey(participantId);

  // Periodically rotate keys (e.g., every 5 minutes)
  setInterval(async () => {
    try {
      const newKey = await e2eeManager.ratchetKey(participantId);
      console.log('Key rotated for participant:', participantId, newKey);

      // Notify other participants about key rotation
      // (implement your own key distribution mechanism)
    } catch (error) {
      console.error('Failed to rotate key:', error);
    }
  }, 5 * 60 * 1000);
}

/**
 * Example 4: Export/Import keys for persistence
 */
async function keyPersistenceExample() {
  const keyProvider = new DefaultKeyProvider();
  const participantId = 'participant-123';

  // Generate key
  const key = await keyProvider.generateKey(participantId);

  // Export key to store securely
  const exportedKey = await keyProvider.exportKey(participantId);
  const keyBase64 = btoa(String.fromCharCode(...new Uint8Array(exportedKey)));
  localStorage.setItem(`e2ee-key-${participantId}`, keyBase64);

  // Later, import the key
  const storedKeyBase64 = localStorage.getItem(`e2ee-key-${participantId}`);
  if (storedKeyBase64) {
    const keyData = Uint8Array.from(atob(storedKeyBase64), c => c.charCodeAt(0));
    await keyProvider.importKey(participantId, keyData.buffer);
    console.log('Key imported from storage');
  }
}

/**
 * Example 5: Check browser support
 */
function checkBrowserSupport() {
  if (!E2EEManager.isSupported()) {
    console.error('E2EE is not supported in this browser');
    alert('Your browser does not support E2EE. Please use a modern browser with Insertable Streams API support.');
    return false;
  }
  console.log('E2EE is supported');
  return true;
}

/**
 * Example 6: Error handling (fail-closed)
 */
async function errorHandlingExample() {
  const keyProvider = new DefaultKeyProvider();
  const e2eeManager = new E2EEManager({
    enabled: true,
    keyProvider,
  });

  // Ensure a key exists before encrypting
  await keyProvider.generateKey('participant-1');

  const sender = {} as RTCRtpSender; // Mock sender
  await e2eeManager.setupSenderTransform(sender, 'track-1', 'participant-1');

  // If encryption fails at runtime, frames are dropped (no plaintext fallback).
}

// Run examples
if (typeof window !== 'undefined') {
  // Check support first
  if (checkBrowserSupport()) {
    console.log('E2EE examples ready to run');
    // Uncomment to run specific examples:
    // basicE2EEExample();
    // customKeyProviderExample();
    // keyRotationExample();
    // keyPersistenceExample();
  }
}
