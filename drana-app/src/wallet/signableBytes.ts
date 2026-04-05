import { sha256 } from '@noble/hashes/sha256';
import { toHex, fromHex, parseAddress } from './crypto';

/**
 * Produces identical bytes to Go's Transaction.SignableBytes().
 * Field order and encoding MUST match exactly.
 */
export function computeSignableBytes(tx: {
  type: number;
  sender: string;      // drana1...
  recipient: string;   // drana1... or empty
  postId: string;      // 64-char hex or empty
  text: string;
  channel: string;
  amount: number;
  nonce: number;
  pubKey: string;      // 64-char hex
}): Uint8Array {
  const parts: Uint8Array[] = [];

  // WriteUint32(type)
  parts.push(writeUint32(tx.type));
  // WriteBytes(sender) — 24 bytes
  parts.push(writeLenPrefixed(parseAddress(tx.sender)));
  // WriteBytes(recipient) — 24 bytes (zero if empty)
  parts.push(writeLenPrefixed(tx.recipient ? parseAddress(tx.recipient) : new Uint8Array(24)));
  // WriteBytes(postId) — 32 bytes (zero if empty)
  parts.push(writeLenPrefixed(tx.postId ? fromHex(tx.postId) : new Uint8Array(32)));
  // WriteString(text)
  parts.push(writeLenPrefixed(new TextEncoder().encode(tx.text)));
  // WriteString(channel)
  parts.push(writeLenPrefixed(new TextEncoder().encode(tx.channel)));
  // WriteUint64(amount)
  parts.push(writeUint64(tx.amount));
  // WriteUint64(nonce)
  parts.push(writeUint64(tx.nonce));
  // WriteBytes(pubKey) — 32 bytes
  parts.push(writeLenPrefixed(fromHex(tx.pubKey)));

  // Concatenate all parts.
  const totalLen = parts.reduce((sum, p) => sum + p.length, 0);
  const result = new Uint8Array(totalLen);
  let offset = 0;
  for (const part of parts) {
    result.set(part, offset);
    offset += part.length;
  }
  return result;
}

/**
 * Computes the transaction hash: SHA-256(WriteBytes(signableBytes) + WriteBytes(signature))
 */
export function computeTxHash(tx: {
  type: number;
  sender: string;
  recipient: string;
  postId: string;
  text: string;
  channel: string;
  amount: number;
  nonce: number;
  pubKey: string;
  signature: string; // hex
}): string {
  const signable = computeSignableBytes(tx);
  const sig = fromHex(tx.signature);

  // Go's Hash() does: hw.WriteBytes(signableBytes); hw.WriteBytes(signature); hw.Sum256()
  const parts: Uint8Array[] = [];
  parts.push(writeLenPrefixed(signable));
  parts.push(writeLenPrefixed(sig));

  const totalLen = parts.reduce((sum, p) => sum + p.length, 0);
  const buf = new Uint8Array(totalLen);
  let offset = 0;
  for (const part of parts) {
    buf.set(part, offset);
    offset += part.length;
  }
  return toHex(sha256(buf));
}

/**
 * Derives a PostID: SHA-256(SHA-256(WriteBytes(addressBytes) + WriteUint64(nonce)))
 * Must match Go's DerivePostID.
 */
export function derivePostID(authorAddress: string, nonce: number): string {
  // Go does: hw.WriteBytes(author[:]); hw.WriteUint64(nonce); SHA-256(hw.buf)
  // hw.WriteBytes = 8-byte len prefix + data
  const addrBytes = parseAddress(authorAddress);
  const parts: Uint8Array[] = [];
  parts.push(writeLenPrefixed(addrBytes));
  parts.push(writeUint64(nonce));

  const totalLen = parts.reduce((sum, p) => sum + p.length, 0);
  const buf = new Uint8Array(totalLen);
  let offset = 0;
  for (const part of parts) {
    buf.set(part, offset);
    offset += part.length;
  }
  return toHex(sha256(buf));
}

// --- encoding helpers (match Go's HashWriter) ---

function writeUint32(v: number): Uint8Array {
  const buf = new Uint8Array(4);
  const dv = new DataView(buf.buffer);
  dv.setUint32(0, v, false); // big-endian
  return buf;
}

function writeUint64(v: number | bigint): Uint8Array {
  const buf = new Uint8Array(8);
  const dv = new DataView(buf.buffer);
  const big = typeof v === 'bigint' ? v : BigInt(v);
  dv.setBigUint64(0, big, false); // big-endian
  return buf;
}

function writeLenPrefixed(data: Uint8Array): Uint8Array {
  const lenBuf = writeUint64(data.length);
  const result = new Uint8Array(8 + data.length);
  result.set(lenBuf, 0);
  result.set(data, 8);
  return result;
}
