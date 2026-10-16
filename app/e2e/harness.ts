// App test harness (docs/plans/0004-m2-ui-read-path.md, Phase 4): a canary
// HTTP server that records every request, fixture data directories built by
// tools/uifixture, and the built app driven through WebKitWebDriver against a
// maild on that data. Run with make ui-e2e, which builds the binaries first.
// On Windows (CI's Windows app job) the app runs in WebView2, driven by
// msedgedriver attached to it (docs/design/testing.md).
import { type ChildProcess, execFileSync, spawn, spawnSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createServer as createHTTPServer } from "node:http";
import { connect as connectNet, createServer as createNetServer } from "node:net";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

import { Session } from "./webdriver";

/** ROOT is the repository; BUILD holds maild, uifixture and frostmail-app. */
export const ROOT = resolve(import.meta.dirname, "../..");
export const BUILD = join(ROOT, "build");

/** WINDOWS is true when the app under test runs in WebView2. */
export const WINDOWS = process.platform === "win32";

/** bin is a program in BUILD, with Windows' .exe. */
export function bin(name: string): string {
  return join(BUILD, WINDOWS ? `${name}.exe` : name);
}

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
  const proc = spawn(bin("smtpsink"), ["-dir", out], { stdio: ["ignore", "pipe", "inherit"] });
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

/** Opener stands in for the launchers the app shell opens links with. */
export interface Opener {
  /** env puts the stand-ins first on the app's PATH; pass it to launch. */
  env: Record<string, string>;
  /** opened returns the URLs the app asked to open, oldest first. */
  opened: () => string[];
}

/**
 * fakeOpener writes xdg-open and gio stand-ins into dir/bin that record
 * their last argument. Windows opens through the shell, not a program on
 * PATH, so there the app logs what it would open (FROSTMAIL_OPEN_LOG).
 */
export function fakeOpener(dir: string): Opener {
  const stand = join(dir, "bin");
  const log = join(dir, "opened");
  const opened = () => (existsSync(log) ? readFileSync(log, "utf8").split(/\r?\n/).filter(Boolean) : []);
  if (WINDOWS) return { env: { FROSTMAIL_OPEN_LOG: log }, opened };
  mkdirSync(stand, { recursive: true });
  for (const name of ["xdg-open", "gio"]) {
    writeFileSync(join(stand, name), `#!/bin/sh\nfor a; do last=$a; done\nprintf '%s\\n' "$last" >> '${log}'\n`, {
      mode: 0o755,
    });
  }
  return { env: { PATH: `${stand}:${process.env.PATH ?? ""}` }, opened };
}

/** FixtureOptions add to the fixture: the hostile corpus pointed at a canary, an SMTP server to send through, or other HTML messages for the Hostile mailbox; or replace it with a showcase directory (tools/uifixture -showcase). */
export interface FixtureOptions {
  canaryOrigin?: string;
  smtp?: string;
  html?: { name: string; html: string }[];
  showcase?: string;
}

/** fixture builds a maild data directory in dir with n messages and the options' extras. */
export function fixture(dir: string, n: number, opts: FixtureOptions = {}): string {
  const { canaryOrigin, smtp, html = [], showcase } = opts;
  const args = ["-out", join(dir, "data"), "-n", String(n)];
  if (smtp) args.push("-smtp", smtp);
  if (showcase) args.push("-showcase", showcase);
  if (canaryOrigin || html.length > 0) {
    const copy = join(dir, "hostile");
    mkdirSync(copy, { recursive: true });
    const files = [...(canaryOrigin ? hostileFiles(canaryOrigin) : []), ...html];
    for (const f of files) writeFileSync(join(copy, f.name), f.html);
    args.push("-hostile", copy);
  }
  execFileSync(bin("uifixture"), args, { stdio: "inherit" });
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

/** pipeUp reports whether a named pipe accepts a connection. */
function pipeUp(path: string): Promise<boolean> {
  return new Promise((done) => {
    const c = connectNet(path);
    c.once("connect", () => {
      c.destroy();
      done(true);
    });
    c.once("error", () => done(false));
  });
}

/** reachable reports whether an HTTP endpoint answers with success. */
async function reachable(url: string): Promise<boolean> {
  try {
    return (await fetch(url)).ok;
  } catch {
    return false;
  }
}

/**
 * startApp starts the app and the WebDriver server that drives it on port,
 * adding them to procs as they start, and returns the session's
 * capabilities.
 *
 * WebKitWebDriver launches the app itself; Tauri allows automation when
 * TAURI_WEBVIEW_AUTOMATION is set (all tauri-driver does on Linux).
 * msedgedriver, which would look for the WebView2 DevTools port in a data
 * folder of its own while Tauri gives WebView2 the app's, attaches instead
 * to the port the app opens when FROSTMAIL_DEVTOOLS_PORT names one.
 */
async function startApp(port: number, env: NodeJS.ProcessEnv, procs: ChildProcess[]): Promise<Record<string, unknown>> {
  const stdio: ["ignore", "inherit", "inherit"] = ["ignore", "inherit", "inherit"];
  if (!WINDOWS) {
    procs.push(
      spawn("WebKitWebDriver", [`--port=${port}`], { env: { ...env, TAURI_WEBVIEW_AUTOMATION: "true" }, stdio }),
    );
    return { "webkitgtk:browserOptions": { binary: bin("frostmail-app"), args: ["--automation"] } };
  }
  const devtools = await freePort();
  const app = spawn(bin("frostmail-app"), [], {
    env: { ...env, FROSTMAIL_DEVTOOLS_PORT: String(devtools) },
    stdio: ["ignore", "pipe", "pipe"],
  });
  procs.push(app);
  let output = "";
  for (const stream of [app.stdout, app.stderr]) {
    stream?.on("data", (chunk: Buffer) => {
      output += chunk.toString();
      process.stderr.write(chunk);
    });
  }
  try {
    await until("the app's DevTools port", () => reachable(`http://127.0.0.1:${devtools}/json/version`), 60_000);
  } catch (err) {
    const state = app.exitCode === null ? "is still running" : `exited with ${app.exitCode}`;
    throw new Error(`${String(err)}; the app ${state}${output ? `: ${output.trim()}` : ""}`);
  }
  procs.push(spawn(process.env.FROSTMAIL_EDGEDRIVER ?? "msedgedriver.exe", [`--port=${port}`], { env, stdio }));
  return { browserName: "webview2", "ms:edgeOptions": { debuggerAddress: `127.0.0.1:${devtools}` } };
}

/** stopProcess ends a process and, on Windows, what it started (the app's WebView2 processes). */
function stopProcess(p: ChildProcess): Promise<void> {
  return new Promise<void>((done) => {
    if (p.exitCode !== null || p.signalCode !== null) return done();
    p.once("exit", () => done());
    if (WINDOWS && p.pid !== undefined) spawnSync("taskkill", ["/T", "/F", "/PID", String(p.pid)], { stdio: "ignore" });
    else p.kill("SIGTERM");
  });
}

/** launch starts maild on the data directory (with extra environment) and the app through WebDriver. */
export async function launch(dir: string, data: string, extraEnv: Record<string, string> = {}): Promise<App> {
  const cache = join(dir, "cache");
  const run = join(dir, "run");
  mkdirSync(cache, { recursive: true, mode: 0o700 });
  mkdirSync(run, { recursive: true, mode: 0o700 });
  const socket = WINDOWS ? `\\\\.\\pipe\\frostmail-e2e-${randomBytes(8).toString("hex")}` : join(run, "maild.sock");
  const env = {
    ...process.env,
    FROSTMAIL_DATA_DIR: data,
    FROSTMAIL_CACHE_DIR: cache,
    FROSTMAIL_SOCKET: socket,
    ...extraEnv,
  };
  const procs: ChildProcess[] = [];
  let session: Session | undefined;
  const stop = async () => {
    await session?.quit().catch(() => {});
    await Promise.all(procs.reverse().map(stopProcess));
    // The run's data (fixtures reach 500 MB) lives in the machine's small
    // /tmp. Windows may hold a file of a process just ended for a moment.
    rmSync(dir, { recursive: true, force: true, maxRetries: 10 });
  };
  try {
    procs.push(spawn(bin("maild"), [], { env, stdio: ["ignore", "inherit", "inherit"] }));
    await until("maild's socket", () => (WINDOWS ? pipeUp(socket) : existsSync(socket)));
    const port = await freePort();
    const capabilities = await startApp(port, env, procs);
    const url = `http://127.0.0.1:${port}`;
    await until("the WebDriver server", () => reachable(`${url}/status`));
    session = await Session.start(url, capabilities);
    return { session, cache, stop };
  } catch (err) {
    // A single-instance app left running would take the next launch.
    await stop();
    throw err;
  }
}
