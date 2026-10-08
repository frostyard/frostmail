// App test harness (docs/plans/0004-m2-ui-read-path.md, Phase 4): a canary
// HTTP server that records every request, fixture data directories built by
// tools/uifixture, and the built app driven through WebKitWebDriver against a
// maild on that data. Run with make ui-e2e, which builds the binaries first.
import { type ChildProcess, execFileSync, spawn } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
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

/** Sink is a running tools/smtpsink. */
export interface Sink {
  addr: string;
  /** messages returns the accepted messages, oldest first. */
  messages: () => string[];
  stop: () => void;
}

/** smtpSink starts tools/smtpsink, writing into dir/sink. */
export async function smtpSink(dir: string): Promise<Sink> {
  const out = join(dir, "sink");
  const proc = spawn(join(BUILD, "smtpsink"), ["-dir", out], { stdio: ["ignore", "pipe", "inherit"] });
  const addr = await new Promise<string>((done, fail) => {
    let buf = "";
    proc.stdout?.on("data", (chunk: Buffer) => {
      buf += chunk.toString();
      const m = /listening (\S+)/.exec(buf);
      if (m?.[1]) done(m[1]);
    });
    proc.on("exit", (code) => fail(new Error(`smtpsink exited with ${code}`)));
  });
  return {
    addr,
    messages: () =>
      existsSync(out)
        ? readdirSync(out)
            .filter((f) => f.endsWith(".eml"))
            .sort((a, b) => Number.parseInt(a, 10) - Number.parseInt(b, 10))
            .map((f) => readFileSync(join(out, f), "utf8"))
        : [],
    stop: () => proc.kill("SIGTERM"),
  };
}

/** FixtureOptions add to the fixture: the hostile corpus pointed at a canary, an SMTP server to send through. */
export interface FixtureOptions {
  canaryOrigin?: string;
  smtp?: string;
}

/** fixture builds a maild data directory in dir with n messages and the options' extras. */
export function fixture(dir: string, n: number, opts: FixtureOptions = {}): string {
  const { canaryOrigin, smtp } = opts;
  const args = ["-out", join(dir, "data"), "-n", String(n)];
  if (smtp) args.push("-smtp", smtp);
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

/** App is a running maild plus the app in a WebDriver session; stop also deletes its directory. */
export interface App {
  session: Session;
  cache: string;
  stop: () => Promise<void>;
}

/** launch starts maild on the data directory (with extra environment) and the app through WebKitWebDriver. */
export async function launch(dir: string, data: string, extraEnv: Record<string, string> = {}): Promise<App> {
  const cache = join(dir, "cache");
  const run = join(dir, "run");
  mkdirSync(cache, { recursive: true, mode: 0o700 });
  mkdirSync(run, { recursive: true, mode: 0o700 });
  const env = {
    ...process.env,
    FROSTMAIL_DATA_DIR: data,
    FROSTMAIL_CACHE_DIR: cache,
    FROSTMAIL_SOCKET: join(run, "maild.sock"),
    ...extraEnv,
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
      await Promise.all(
        procs.reverse().map(
          (p) =>
            new Promise<void>((done) => {
              if (p.exitCode !== null) return done();
              p.once("exit", () => done());
              p.kill("SIGTERM");
            }),
        ),
      );
      // The run's data (fixtures reach 500 MB) lives in the machine's small /tmp.
      rmSync(dir, { recursive: true, force: true });
    },
  };
}
