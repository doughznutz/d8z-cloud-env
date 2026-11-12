import { userCount, createUser } from "./users.js";
import { loadConfig } from "../config.js";

/**
 * bootstrapAdmin seeds a first admin user from CN_ADMIN_USER/CN_ADMIN_PASSWORD
 * when the users table is empty - the same idea as entrypoint.sh seeding
 * Traefik's basic-auth htpasswd from DOUGHZNUTZ_USER/PASSWORD, so there's a
 * way to log in at all on a fresh database.
 */
export function bootstrapAdmin(): void {
  if (userCount() > 0) return;
  const cfg = loadConfig();
  if (!cfg.adminUser || !cfg.adminPassword) {
    console.warn("console-next: no users exist and CN_ADMIN_USER/CN_ADMIN_PASSWORD are unset - no one can log in yet");
    return;
  }
  createUser(cfg.adminUser, cfg.adminPassword, true);
  console.log(`console-next: seeded initial admin user "${cfg.adminUser}"`);
}
