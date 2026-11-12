// The symmetric key console-next uses to encrypt credentials.secret_encrypted
// at rest (src/crypto/secretBox.ts). Generated on first run, 0600, entirely
// separate from Go's wsnats.go key material - different purpose, different
// trust boundary, never shared.

import { randomBytes } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, writeFileSync, chmodSync } from "node:fs";
import { dirname } from "node:path";
import { loadConfig } from "../config.js";

let cached: Buffer | undefined;

/** masterKey returns console-next's own AES-256-GCM key, generating and persisting one on first use. */
export function masterKey(): Buffer {
  if (cached) return cached;
  const cfg = loadConfig();
  if (existsSync(cfg.masterKeyFile)) {
    cached = Buffer.from(readFileSync(cfg.masterKeyFile, "utf8").trim(), "hex");
    return cached;
  }
  mkdirSync(dirname(cfg.masterKeyFile), { recursive: true });
  const key = randomBytes(32);
  writeFileSync(cfg.masterKeyFile, key.toString("hex"), { mode: 0o600 });
  try {
    chmodSync(cfg.masterKeyFile, 0o600);
  } catch {
    // Best effort - e.g. not owned by us on some mounts; not fatal.
  }
  cached = key;
  return cached;
}
