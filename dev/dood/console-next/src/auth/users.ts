// User accounts: scrypt-hashed passwords (node:crypto, no bcrypt
// dependency - same "stdlib crypto, nothing extra" approach as Go's own
// wsnats.go), stored in console-next's SQLite file (src/db.ts).

import { randomBytes, scryptSync, timingSafeEqual } from "node:crypto";
import { db } from "../db.js";

/** User is console-next's view of a logged-in account - no password material. */
export interface User {
  id: number;
  username: string;
  isAdmin: boolean;
}

interface UserRow {
  id: number;
  username: string;
  password_hash: string;
  is_admin: number;
}

const SCRYPT_KEYLEN = 64;

/** hashPassword derives a salted scrypt hash, stored as "<saltHex>:<hashHex>". */
function hashPassword(password: string): string {
  const salt = randomBytes(16);
  const hash = scryptSync(password, salt, SCRYPT_KEYLEN);
  return `${salt.toString("hex")}:${hash.toString("hex")}`;
}

/** verifyPasswordHash checks password against a "<saltHex>:<hashHex>" hash in constant time. */
function verifyPasswordHash(password: string, stored: string): boolean {
  const [saltHex, hashHex] = stored.split(":");
  if (!saltHex || !hashHex) return false;
  const salt = Buffer.from(saltHex, "hex");
  const expected = Buffer.from(hashHex, "hex");
  const actual = scryptSync(password, salt, expected.length);
  return actual.length === expected.length && timingSafeEqual(actual, expected);
}

function toUser(row: Pick<UserRow, "id" | "username" | "is_admin">): User {
  return { id: row.id, username: row.username, isAdmin: Number(row.is_admin) === 1 };
}

/** createUser adds a new user with a freshly hashed password. Throws if the username is taken. */
export function createUser(username: string, password: string, isAdmin = false): User {
  db.prepare("INSERT INTO users (username, password_hash, is_admin) VALUES (?, ?, ?)").run(username, hashPassword(password), isAdmin ? 1 : 0);
  const row = db.prepare("SELECT id, username, is_admin FROM users WHERE username = ?").get(username) as Pick<UserRow, "id" | "username" | "is_admin">;
  return toUser(row);
}

/** verifyLogin checks a username/password pair against the stored hash, returning the User on success. */
export function verifyLogin(username: string, password: string): User | undefined {
  const row = db.prepare("SELECT id, username, password_hash, is_admin FROM users WHERE username = ?").get(username) as UserRow | undefined;
  if (!row || !verifyPasswordHash(password, row.password_hash)) return undefined;
  return toUser(row);
}

/** getUserByUsername looks up a user by name, for admin-only operations that act on another account. */
export function getUserByUsername(username: string): User | undefined {
  const row = db.prepare("SELECT id, username, is_admin FROM users WHERE username = ?").get(username) as Pick<UserRow, "id" | "username" | "is_admin"> | undefined;
  return row ? toUser(row) : undefined;
}

/** userCount returns how many users exist, used to decide whether to seed a default admin on first run. */
export function userCount(): number {
  const row = db.prepare("SELECT COUNT(*) AS n FROM users").get() as { n: number };
  return Number(row.n);
}
