import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// base "./": the same build is served at `/` (root gateway) and at
// `/gw/<name>/...` (children), so every URL must be relative.
//
// `npm run dev` proxies the API and sub-apps to a running gateway:
//   GW_TARGET=http://user:pass@localhost:8080 npm run dev
const target = process.env.GW_TARGET || "http://localhost:8080";
const proxied = ["/api", "/deightz", "/og", "/dozzle", "/gw"];

export default defineConfig({
  base: "./",
  plugins: [react()],
  server: {
    proxy: Object.fromEntries(proxied.map((p) => [p, { target, changeOrigin: true, ws: true }])),
  },
});
