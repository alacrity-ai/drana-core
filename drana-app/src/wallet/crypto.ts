import * as ed from '@noble/ed25519';
import { sha512 } from '@noble/hashes/sha512';
import { sha256 } from '@noble/hashes/sha256';

// @noble/ed25519 v2 requires sha512 to be configured.
ed.etc.sha512Sync = (...m: Uint8Array[]) => {
  const h = sha512.create();
  for (const b of m) h.update(b);
  return h.digest();
};

// Key generation
export function generateKeyPair() {
  const seed = ed.utils.randomPrivateKey(); // 32 bytes
  const publicKey = ed.getPublicKey(seed);
  // Full 64-byte private key: seed || publicKey (matches Go's ed25519 format)
  const privateKey = new Uint8Array(64);
  privateKey.set(seed, 0);
  privateKey.set(publicKey, 32);
  return { publicKey, privateKey };
}

// Signing
export function sign(privateKey: Uint8Array, message: Uint8Array): Uint8Array {
  const seed = privateKey.slice(0, 32);
  return ed.sign(message, seed);
}

// Address derivation — must match Go exactly
export function deriveAddress(publicKey: Uint8Array): string {
  const pubKeyHash = sha256(publicKey);
  const body = pubKeyHash.slice(0, 20);
  const checksumFull = sha256(body);
  const checksum = checksumFull.slice(0, 4);
  const addrBytes = new Uint8Array(24);
  addrBytes.set(body, 0);
  addrBytes.set(checksum, 20);
  return 'drana1' + toHex(addrBytes);
}

// Parse drana1... address to 24 bytes
export function parseAddress(addr: string): Uint8Array {
  if (!addr.startsWith('drana1')) throw new Error('Invalid address prefix');
  const hex = addr.slice(6);
  if (hex.length !== 48) throw new Error('Invalid address length');
  return fromHex(hex);
}

// AES-GCM encryption for private key storage
export type EncryptedKey = { ciphertext: string; salt: string; iv: string };

export async function encryptPrivateKey(privateKey: Uint8Array, password: string): Promise<EncryptedKey> {
  const salt = crypto.getRandomValues(new Uint8Array(16));
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const keyMaterial = await crypto.subtle.importKey('raw', new TextEncoder().encode(password), 'PBKDF2', false, ['deriveKey']);
  const aesKey = await crypto.subtle.deriveKey(
    { name: 'PBKDF2', salt, iterations: 100000, hash: 'SHA-256' },
    keyMaterial,
    { name: 'AES-GCM', length: 256 },
    false,
    ['encrypt'],
  );
  const buf = new Uint8Array(privateKey).buffer as ArrayBuffer;
  const ciphertext = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, aesKey, buf);
  return {
    ciphertext: btoa(String.fromCharCode(...new Uint8Array(ciphertext))),
    salt: btoa(String.fromCharCode(...salt)),
    iv: btoa(String.fromCharCode(...iv)),
  };
}

export async function decryptPrivateKey(encrypted: EncryptedKey, password: string): Promise<Uint8Array> {
  const salt = Uint8Array.from(atob(encrypted.salt), c => c.charCodeAt(0));
  const iv = Uint8Array.from(atob(encrypted.iv), c => c.charCodeAt(0));
  const ciphertext = Uint8Array.from(atob(encrypted.ciphertext), c => c.charCodeAt(0));
  const keyMaterial = await crypto.subtle.importKey('raw', new TextEncoder().encode(password), 'PBKDF2', false, ['deriveKey']);
  const aesKey = await crypto.subtle.deriveKey(
    { name: 'PBKDF2', salt, iterations: 100000, hash: 'SHA-256' },
    keyMaterial,
    { name: 'AES-GCM', length: 256 },
    false,
    ['decrypt'],
  );
  const plaintext = await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, aesKey, ciphertext);
  return new Uint8Array(plaintext);
}

// Hex utilities
export function toHex(bytes: Uint8Array): string {
  return Array.from(bytes).map(b => b.toString(16).padStart(2, '0')).join('');
}

export function fromHex(hex: string): Uint8Array {
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < hex.length; i += 2) {
    bytes[i / 2] = parseInt(hex.slice(i, i + 2), 16);
  }
  return bytes;
}
