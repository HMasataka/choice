/**
 * E2EE (End-to-End Encryption) module
 * Provides encryption and decryption capabilities for WebRTC media using Insertable Streams API
 */

export { E2EEManager } from './E2EEManager';
export { FrameCryptor } from './FrameCryptor';
export { DefaultKeyProvider } from './KeyProvider';
export type { E2EEKeyProvider } from '../utils/types';
