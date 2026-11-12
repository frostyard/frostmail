/// <reference types="vitest/config" />
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  clearScreen: false,
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
