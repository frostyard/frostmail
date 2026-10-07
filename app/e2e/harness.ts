// App test harness (docs/plans/0004-m2-ui-read-path.md, Phase 4): a canary
// HTTP server that records every request, fixture data directories built by
// tools/uifixture, and the built app driven through WebKitWebDriver against a
// maild on that data. Run with make ui-e2e, which builds the binaries first.
import { type ChildProcess, execFileSync, spawn } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { createServer as createHTTPServer } from "node:http";
import { createServer as createNetServer } from "node:net";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

import { Session } from "./webdriver";

/** ROOT is the repository; BUILD holds maild, uifixture and frostmail-app. */
export const ROOT = resolve(import.meta.dirname, "../..");
export const BUILD = join(ROOT, "build");

/** Canary records every HTTP request made to it. */
export interface Canary {
  origin: string;
  hits: string[];
  close: () => Promise<void>;
}

/** canary starts a recording HTTP server on a free loopback port. */
export function canary(): Promise<Canary> {
  const hits: string[] = [];
  const server = createHTTPServer((req, res) => {
    hits.push(`${req.method} ${req.url}`);
    res.writeHead(204);
    res.end();
  });
  return new Promise((done) => {
    server.listen(0, "127.0.0.1", () => {
      const addr = server.address();
      const port = typeof addr === "object" && addr ? addr.port : 0;
      done({
        origin: `http://127.0.0.1:${port}`,
        hits,
        close: () => new Promise((r) => server.close(() => r())),
      });
    });
  });
}

/** tempDir makes a private directory for one test run. */
export function tempDir(prefix: string): string {
  return mkdtempSync(join(tmpdir(), `frostmail-${prefix}-`));
}

/** HOSTILE is the hostile-HTML corpus. */
export const HOSTILE = join(ROOT, "internal/render/testdata/hostile");

/** hostileFiles lists the corpus with its canary.test URLs pointed at origin. */
export function hostileFiles(origin: string): { name: string; html: string }[] {
  return readdirSync(HOSTILE)
    .filter((f) => f.endsWith(".html"))
    .sort()
    .map((name) => ({
      name,
      html: readFileSync(join(HOSTILE, name), "utf8")
        .replaceAll("http://canary.test", origin)
        .replaceAll("//canary.test", origin.replace("http:", "")),
    }));
}

/** fixture builds a maild data directory in dir with n messages and, if given, the hostile corpus. */
export function fixture(dir: string, n: number, canaryOrigin?: string): string {
  const args = ["-out", join(dir, "data"), "-n", String(n)];
  if (canaryOrigin) {
    const copy = join(dir, "hostile");
    mkdirSync(copy, { recursive: true });
    for (const f of hostileFiles(canaryOrigin)) writeFileSync(join(copy, f.name), f.html);
    args.push("-hostile", copy);
  }
  execFileSync(join(BUILD, "uifixture"), args, { stdio: "inherit" });
  return join(dir, "data");
}

function freePort(): Promise<number> {
  return new Promise((done, fail) => {
    const s = createNetServer();
    s.on("error", fail);
    s.listen(0, "127.0.0.1", () => {
      const addr = s.address();
      const port = typeof addr === "object" && addr ? addr.port : 0;
      s.close(() => done(port));
    });
  });
}

async function until(what: string, ok: () => boolean | Promise<boolean>, timeoutMs = 20_000): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (!(await ok())) {
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await new Promise((r) => setTimeout(r, 100));
  }
}

/** App is a running maild plus the app in a WebDriver session. */
export interface App {
  session: Session;
  cache: string;
  stop: () => Promise<void>;
}

/** launch starts maild on the data directory and the app through WebKitWebDriver. */
export async function launch(dir: string, data: string): Promise<App> {
  const cache = join(dir, "cache");
  const run = join(dir, "run");
  mkdirSync(cache, { recursive: true, mode: 0o700 });
  mkdirSync(run, { recursive: true, mode: 0o700 });
  const env = {
    ...process.env,
    FROSTMAIL_DATA_DIR: data,
    FROSTMAIL_CACHE_DIR: cache,
    FROSTMAIL_SOCKET: join(run, "maild.sock"),
  };
  const procs: ChildProcess[] = [];
  const maild = spawn(join(BUILD, "maild"), [], { env, stdio: ["ignore", "inherit", "inherit"] });
  procs.push(maild);
  await until("maild's socket", () => existsSync(env.FROSTMAIL_SOCKET));

  const port = await freePort();
  // WebKitWebDriver launches the app; Tauri allows automation when
  // TAURI_WEBVIEW_AUTOMATION is set (what tauri-driver does on Linux).
  const driver = spawn("WebKitWebDriver", [`--port=${port}`], {
    env: { ...env, TAURI_WEBVIEW_AUTOMATION: "true" },
    stdio: ["ignore", "inherit", "inherit"],
  });
  procs.push(driver);
  const url = `http://127.0.0.1:${port}`;
  await until("WebKitWebDriver", async () => {
    try {
      return (await fetch(`${url}/status`)).ok;
    } catch {
      return false;
    }
  });
  const session = await Session.start(url, {
    "webkitgtk:browserOptions": { binary: join(BUILD, "frostmail-app"), args: ["--automation"] },
  });
  return {
    session,
    cache,
    stop: async () => {
      await session.quit().catch(() => {});
      for (const p of procs.reverse()) p.kill("SIGTERM");
    },
  };
}
