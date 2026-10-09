/// <reference types="vitest/config" />
import { createReadStream, statSync } from "node:fs";
import { homedir } from "node:os";
import { extname, join, sep } from "node:path";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig, type Plugin } from "vite";

// Inside nsl machines, host file edits raise no inotify events, so the
// watcher polls (docs/adr/0006-development-environment.md).
const poll = process.env.NSL === "1";

const MIME: Record<string, string> = {
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".jpeg": "image/jpeg",
  ".gif": "image/gif",
  ".webp": "image/webp",
  ".bmp": "image/bmp",
  ".ico": "image/x-icon",
  ".avif": "image/avif",
  ".pdf": "application/pdf",
  ".txt": "text/plain; charset=utf-8",
};

// WebKitGTK will not load the local mailpart: scheme into the dev server's
// http page, so in dev the same files are served under /mailpart/ with the
// protocol's rules (app/src-tauri/src/protocol.rs; docs/design/app.md).
function mailpartDev(): Plugin {
  const cache =
    process.env.FROSTMAIL_CACHE_DIR ??
    (process.env.XDG_CACHE_HOME
      ? join(process.env.XDG_CACHE_HOME, "frostmail")
      : join(homedir(), ".cache", "frostmail"));
  const root = join(cache, "parts");
  return {
    name: "frostmail-mailpart",
    apply: "serve",
    configureServer(server) {
      server.middlewares.use("/mailpart", (req, res) => {
        const rel = (req.url ?? "/").split("?")[0]?.replace(/^\/+/, "") ?? "";
        const parts = rel.split("/");
        if (!/^[A-Za-z0-9._/-]+$/.test(rel) || parts.some((p) => p === "" || p === "." || p === "..")) {
          res.statusCode = 400;
          res.end();
          return;
        }
        const file = join(root, rel);
        let ok = false;
        try {
          ok = file.startsWith(root + sep) && statSync(file).isFile();
        } catch {
          ok = false;
        }
        if (!ok) {
          res.statusCode = 404;
          res.end();
          return;
        }
        res.setHeader("Content-Type", MIME[extname(file).toLowerCase()] ?? "application/octet-stream");
        res.setHeader("X-Content-Type-Options", "nosniff");
        res.setHeader(
          "Content-Security-Policy",
          "default-src 'none'; img-src data:; style-src 'unsafe-inline'; sandbox",
        );
        createReadStream(file).pipe(res);
      });
    },
  };
}

export default defineConfig({
  plugins: [react(), tailwindcss(), mailpartDev()],
  clearScreen: false,
  envPrefix: ["VITE_", "TAURI_ENV_"],
  server: {
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
    watch: poll ? { usePolling: true, interval: 300 } : undefined,
  },
  build: { target: "es2022" },
  test: {
    environment: "happy-dom",
    // Tests read dates in UTC wherever they run (CI already does).
    env: { TZ: "UTC" },
    // Room for the integration tests' longer waits (src/testing/setup.ts).
    testTimeout: 20_000,
    include: ["src/**/*.test.{ts,tsx}"],
    setupFiles: ["src/testing/setup.ts"],
  },
});
