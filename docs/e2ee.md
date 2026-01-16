# End-to-End Encryption (E2EE) Integration Guide

## Overview

The SFU Client SDK provides built-in support for End-to-End Encryption (E2EE) using the WebRTC Insertable Streams API (also known as WebRTC Encoded Transform). This ensures that media frames are encrypted on the sender side and decrypted on the receiver side, with the SFU server only forwarding encrypted data without the ability to decrypt it.

## Features

- **Client-side encryption/decryption**: Media is encrypted before leaving the client and decrypted after arrival
- **AES-GCM only**: Authenticated encryption with AAD binding
- **Key management**: Pluggable key provider interface for custom key management
- **Key rotation**: Ratcheted keys are applied immediately to active transforms
- **Replay protection**: Basic frame counter checks drop replays
- **Codec header preservation**: First 10 bytes remain unencrypted for proper codec processing
- **Fail-closed on errors**: Encryption/decryption errors drop frames (no plaintext fallback)

## Browser Support

E2EE requires the Insertable Streams API, which is supported in:
- Chrome/Edge 90+
- Safari 15.4+
- Firefox 117+ (with flag enabled)

Check browser support:

```typescript
import { E2EEManager } from '@sfu/client-sdk';

if (!E2EEManager.isSupported()) {
  console.error('E2EE not supported in this browser');
}
```

## Architecture

### Components

1. **E2EEManager**: Main coordinator for encryption/decryption
2. **FrameCryptor**: Handles frame-level encryption/decryption using WebCrypto API
3. **KeyProvider**: Interface for key management (default implementation provided)

### Flow

```
┌─────────────┐         ┌───────────┐         ┌─────────────┐
│  Publisher  │────────▶│    SFU    │────────▶│ Subscriber  │
│   Client    │         │  Server   │         │   Client    │
└─────────────┘         └───────────┘         └─────────────┘
      │                       │                       │
      │ Encrypt               │ Forward               │ Decrypt
      │ (E2EEManager)         │ (No decrypt)          │ (E2EEManager)
      ▼                       ▼                       ▼
  Encrypted Frame      Encrypted Frame        Decrypted Frame
```

## Configuration

### Server-side

Enable E2EE support in `configs/config.yaml`:

```yaml
media:
  e2ee:
    enabled: true  # Allow E2EE connections
```

Note: The server does not decrypt media. This flag only indicates E2EE support is enabled.

### Client-side

Configure E2EE in the SFU Client:

```typescript
import { SFUClient, DefaultKeyProvider } from '@sfu/client-sdk';

const client = new SFUClient({
  url: 'wss://sfu.example.com/ws',
  e2ee: {
    enabled: true,
    algorithm: 'AES-GCM',
    ratchetStrategy: 'manual', // 'per-frame', 'per-second', or 'manual'
    keyProvider: new DefaultKeyProvider(), // Optional: custom provider
  },
});
```

## Basic Usage

### 1. Setup E2EE Manager

```typescript
import { E2EEManager, DefaultKeyProvider } from '@sfu/client-sdk';

const keyProvider = new DefaultKeyProvider();
const e2eeManager = new E2EEManager({
  enabled: true,
  keyProvider,
});
```

### 2. Generate Keys

```typescript
// Generate key for local participant
const localParticipantId = 'participant-123';
await keyProvider.generateKey(localParticipantId);

// Generate key for remote participant
const remoteParticipantId = 'participant-456';
await keyProvider.generateKey(remoteParticipantId);
```

### 3. Setup Encryption for Outgoing Tracks

```typescript
// After publishing a track
const sender = peerConnection.getSender(trackId);
await e2eeManager.setupSenderTransform(
  sender,
  trackId,
  localParticipantId
);
```

### 4. Setup Decryption for Incoming Tracks

```typescript
// After subscribing to a track
const receiver = peerConnection.getReceiver(trackId);
await e2eeManager.setupReceiverTransform(
  receiver,
  trackId,
  remoteParticipantId
);
```

## Key Management

### Default Key Provider

The SDK includes a `DefaultKeyProvider` that stores keys in memory:

```typescript
const keyProvider = new DefaultKeyProvider();

// Generate new key
await keyProvider.generateKey(participantId);

// Get key
const key = await keyProvider.getKey(participantId);

// Remove key
await keyProvider.removeKey(participantId);

// Key rotation (HKDF-based)
const newKey = await keyProvider.ratchetKey(participantId);
```

### Custom Key Provider

Implement the `E2EEKeyProvider` interface for custom key management:

```typescript
import type { E2EEKeyProvider } from '@sfu/client-sdk';

class MyKeyProvider implements E2EEKeyProvider {
  async getKey(participantId: string): Promise<CryptoKey | null> {
    // Fetch from your key management service
    return null;
  }

  async setKey(participantId: string, key: CryptoKey): Promise<void> {
    // Store in your key management service
  }

  async removeKey(participantId: string): Promise<void> {
    // Remove from your key management service
  }

  async ratchetKey(participantId: string): Promise<CryptoKey> {
    // Implement key derivation
    throw new Error('Not implemented');
  }
}

const e2eeManager = new E2EEManager({
  enabled: true,
  keyProvider: new MyKeyProvider(),
});
```

## Key Distribution

The SDK **does not handle key distribution** - you must implement this separately. Common approaches:

### 1. Out-of-band Key Exchange

```typescript
// Example: Exchange keys via signaling channel
room.on('participantJoined', async (participant) => {
  // Generate key for this participant
  const key = await keyProvider.generateKey(participant.id);

  // Export key
  const keyData = await keyProvider.exportKey(participant.id);

  // Send key to participant via secure channel
  await sendKeyToParticipant(participant.id, keyData);
});
```

### 2. Shared Secret

```typescript
// All participants use a pre-shared secret (e.g., room password)
const sharedSecret = await deriveKeyFromPassword(roomPassword);
for (const participant of room.participants) {
  await keyProvider.setKey(participant.id, sharedSecret);
}
```

### 3. Public Key Cryptography

```typescript
// Use ECDH for key agreement
const { publicKey, privateKey } = await generateKeyPair();

// Exchange public keys
await broadcastPublicKey(publicKey);

// Derive shared secret
const sharedSecret = await deriveSharedSecret(privateKey, remotePublicKey);
await keyProvider.setKey(remoteParticipantId, sharedSecret);
```

## Key Rotation

Periodically rotate keys for enhanced security (rotations are applied immediately to active transforms):

```typescript
// Rotate key every 5 minutes
setInterval(async () => {
  const newKey = await e2eeManager.ratchetKey(participantId);

  // Notify other participants to also ratchet
  await notifyKeyRotation(participantId);
}, 5 * 60 * 1000);
```

## Frame Format

Each encrypted frame uses the following structure (byte offsets shown):

```
0      : headerLength (1 byte, 0-10)
1..H   : unencryptedHeader (H bytes)
H+1..  : IV (12 bytes)
...    : Metadata (12 bytes: participantHash[4] + frameCounter[4] + keyIndex[4])
...    : Ciphertext (AES-GCM, includes auth tag)
```

Authenticated Additional Data (AAD) binds the cleartext parts:

```
AAD = headerLength + unencryptedHeader + metadata
```

## Frame Metadata

Metadata is included in each encrypted frame:

- **Participant ID hash** (4 bytes): Identifies the sender
- **Frame counter** (4 bytes): Used for replay protection
- **Key index** (4 bytes): Supports key rotation

The metadata is readable by the server but does not reveal encryption keys.

## Security Considerations

### 1. Key Storage

- **Never store keys in plain text**
- Use browser's IndexedDB with encryption
- Consider using Web Crypto's non-extractable keys where possible

### 2. Key Exchange

- Use authenticated channels for key distribution
- Implement perfect forward secrecy (PFS) with periodic key rotation
- Verify participant identities before exchanging keys

### 3. Replay Protection

- The SDK enforces a basic monotonic frame counter check per sender hash
- Frames with non-increasing counters are dropped
- Note: this is basic protection and may drop frames on severe reordering

### 4. Algorithm Selection

- **AES-GCM** (only): Provides both encryption and authentication

## Performance Impact

E2EE adds computational overhead:

- **Encryption**: ~1-5% CPU increase per stream
- **Decryption**: ~1-5% CPU increase per stream
- **Latency**: < 1ms additional latency

Factors affecting performance:
- Number of simultaneous streams
- Resolution and framerate
- Device capabilities

## Troubleshooting

### Keys not found

```typescript
// Ensure keys are generated before setting up transforms
await keyProvider.generateKey(participantId);
await e2eeManager.setupSenderTransform(sender, trackId, participantId);
```

### Decryption failures

- Verify keys match between sender and receiver
- Check that key indices match
- Ensure frames aren't being dropped/reordered excessively
 - Note: failed decryptions are dropped; plaintext is never forwarded

### Browser compatibility

```typescript
if (!E2EEManager.isSupported()) {
  // Fallback to unencrypted or show error
  alert('E2EE requires a modern browser');
}
```

## API Reference

### E2EEManager

```typescript
class E2EEManager {
  constructor(config: E2EEConfig);

  static isSupported(): boolean;

  setLocalParticipantId(participantId: string): void;

  setupSenderTransform(
    sender: RTCRtpSender,
    trackId: string,
    participantId: string
  ): Promise<void>;

  setupReceiverTransform(
    receiver: RTCRtpReceiver,
    trackId: string,
    participantId: string
  ): Promise<void>;

  removeTransform(trackId: string): void;

  setKey(participantId: string, key: CryptoKey): Promise<void>;
  removeKey(participantId: string): Promise<void>;
  ratchetKey(participantId: string): Promise<CryptoKey>;

  enable(): void;
  disable(): void;
  isEnabled(): boolean;

  getAlgorithm(): E2EEAlgorithm;
  getKeyProvider(): E2EEKeyProvider;
}
```

### DefaultKeyProvider

```typescript
class DefaultKeyProvider implements E2EEKeyProvider {
  constructor(algorithm: E2EEAlgorithm);

  generateKey(participantId: string): Promise<CryptoKey>;
  importKey(participantId: string, keyData: ArrayBuffer): Promise<CryptoKey>;
  exportKey(participantId: string): Promise<ArrayBuffer>;

  getKey(participantId: string): Promise<CryptoKey | null>;
  setKey(participantId: string, key: CryptoKey): Promise<void>;
  removeKey(participantId: string): Promise<void>;
  ratchetKey(participantId: string): Promise<CryptoKey>;

  clearAll(): void;
  getKeyCount(): number;
}
```

## Examples

See `sdk/client-sdk/examples/e2ee-example.ts` for complete working examples.

## References

- [WebRTC Insertable Streams API](https://www.w3.org/TR/webrtc-encoded-transform/)
- [Web Crypto API](https://www.w3.org/TR/WebCryptoAPI/)
- [HKDF (RFC 5869)](https://tools.ietf.org/html/rfc5869)
- [AES-GCM](https://csrc.nist.gov/publications/detail/sp/800-38d/final)
