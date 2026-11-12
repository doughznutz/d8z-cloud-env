import type { User } from "./users.js";

/** AppEnv is the Hono environment type: every request past requireAuth carries its User as context variable "user". */
export interface AppEnv {
  Variables: { user: User };
}
