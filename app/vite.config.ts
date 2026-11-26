/// <reference types="vitest/config" />
// The loader of the NSGo client's web build in lodge's frame
// (docs/specs/lodge-app.md, The web build): make web-build builds index.html
// and src/nsgo-lodge.ts for the base /app/mail/, and puts the client's Wasm
// build beside them.
import { defineConfig } from "vite";

export default defineConfig({
  clearScreen: false,
  build: { target: "es2022" },
  test: { include: ["src/**/*.test.ts"] },
});
