import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// Inside nsl machines, host file edits raise no inotify events, so the
// watcher polls (docs/adr/0006-development-environment.md).
const poll = process.env.NSL === "1";

export default defineConfig({
  plugins: [react()],
  clearScreen: false,
  envPrefix: ["VITE_", "TAURI_ENV_"],
  server: {
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
    watch: poll ? { usePolling: true, interval: 300 } : undefined,
  },
  build: { target: "es2022" },
});
