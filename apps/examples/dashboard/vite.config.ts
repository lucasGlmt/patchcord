import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  // Relative, not "/": the agent serves this app under /apps/dashboard/,
  // a subpath Vite doesn't know about at build time. The default base
  // ("/") emits absolute asset URLs (e.g. "/assets/index-xxxx.js") that
  // 404 once installed, since the browser resolves them against the
  // origin, not against /apps/dashboard/index.html. "./" makes every
  // emitted URL relative to index.html itself.
  base: "./",
  plugins: [react()],
});
