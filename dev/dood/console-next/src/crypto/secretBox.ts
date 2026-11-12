// AES-256-GCM encrypt/decrypt for credentials.secret_encrypted, keyed by
// src/crypto/masterKey.ts. Blob format: base64(iv [12B] + authTag [16B] + ciphertext).

import { createCipheriv, createDecipheriv, randomBytes } from "node:crypto";
import { masterKey } from "./masterKey.js";

const IV_LEN = 12;
const TAG_LEN = 16;

/** encryptSecret encrypts plaintext with console-next's master key, returning a self-contained base64 blob. */
export function encryptSecret(plaintext: string): string {
  const iv = randomBytes(IV_LEN);
  const cipher = createCipheriv("aes-256-gcm", masterKey(), iv);
  const ciphertext = Buffer.concat([cipher.update(plaintext, "utf8"), cipher.final()]);
  const tag = cipher.getAuthTag();
  return Buffer.concat([iv, tag, ciphertext]).toString("base64");
}

/** decryptSecret reverses encryptSecret; throws if the blob is malformed or the auth tag doesn't verify. */
export function decryptSecret(blob: string): string {
  const raw = Buffer.from(blob, "base64");
  const iv = raw.subarray(0, IV_LEN);
  const tag = raw.subarray(IV_LEN, IV_LEN + TAG_LEN);
  const ciphertext = raw.subarray(IV_LEN + TAG_LEN);
  const decipher = createDecipheriv("aes-256-gcm", masterKey(), iv);
  decipher.setAuthTag(tag);
  return Buffer.concat([decipher.update(ciphertext), decipher.final()]).toString("utf8");
}
