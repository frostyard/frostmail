// The desktop app on a maild behind lodge (docs/design/desktop.md, On a
// maild behind lodge): lodge's development build on loopback, maild --lodge
// on a fixture, and the app with app.json naming lodge and no maild socket
// of its own. It lists, reads, and sends a reply through lodge. It needs
// lodge's development build: FROSTMAIL_LODGE_E2E names the directory with
// lodge-e2e and lodgectl (lodge's make e2e-build); without it, it skips.
import { type ChildProcess, execFileSync, spawn } from "node:child_process";
import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  type App,
  BUILD,
  bin,
  fixture,
  freePort,
  launchApp,
  ROOT,
  type Sink,
  smtpSink,
  tempDir,
  until,
} from "./harness";
import type { Session } from "./webdriver";

const LODGE = process.env.FROSTMAIL_LODGE_E2E;

let app: App | undefined;
let sink: Sink | undefined;
let maildLog = "";

/** stage puts maild's manifest, units and a placeholder web build under root, as lodge installs them. */
function stage(root: string): void {
  execFileSync("sh", [join(ROOT, "packaging/lodge/stage.sh"), root, "e2e", BUILD], { stdio: "inherit" });
  const web = join(root, "usr/share/lodge/apps/mail/web");
  if (!existsSync(join(web, "index.html"))) {
    mkdirSync(web, { recursive: true });
    writeFileSync(join(web, "index.html"), "<!doctype html><title>Mail</title>");
    writeFileSync(join(web, "icon.svg"), '<svg xmlns="http://www.w3.org/2000/svg"/>');
  }
}

beforeAll(async () => {
  if (!LODGE) return;
  const dir = tempDir("lodge");
  const root = join(dir, "root");
  const run = join(dir, "run");
  mkdirSync(run, { recursive: true, mode: 0o700 });
  stage(root);
  sink = await smtpSink(dir);
  const data = fixture(dir, 20, { smtp: sink.addr });
  const procs: ChildProcess[] = [];
  const port = await freePort();
  const cport = await freePort();
  const ctl = (...args: string[]) =>
    execFileSync(join(LODGE, "lodgectl"), args, { env: { ...process.env, LODGE_ROOT: root }, stdio: "pipe" });

  procs.push(
    spawn(
      join(LODGE, "lodge-e2e"),
      ["--root", root, "--listen", `127.0.0.1:${port}`, "--content-listen", `127.0.0.1:${cport}`],
      { stdio: ["ignore", "inherit", "inherit"] },
    ),
  );
  await until("lodge's control socket", () => existsSync(join(root, "run/lodge/control.sock")));
  ctl("allow", "login", "e2e@example.com");
  ctl("allow", "node", "e2e-node");

  const maild = spawn(bin("maild"), ["--lodge"], {
    env: {
      ...process.env,
      LODGE_ROOT: root,
      FROSTMAIL_DATA_DIR: data,
      FROSTMAIL_CACHE_DIR: join(dir, "cache"),
      FROSTMAIL_SOCKET: join(run, "maild.sock"),
      FROSTMAIL_SECRETS: "file",
      FROSTMAIL_UNDO_DELAY: "1s",
    },
    stdio: ["ignore", "inherit", "pipe"],
  });
  maild.stderr?.on("data", (chunk: Buffer) => {
    maildLog += chunk.toString();
    process.stderr.write(chunk);
  });
  procs.push(maild);
  await until("maild's lodge socket", () => existsSync(join(root, "run/lodge/apps/mail.sock")));
  ctl("reload");

  // The app's own choice of maild, and no local maild to fall back on.
  const config = join(dir, "config");
  mkdirSync(join(config, "frostmail"), { recursive: true });
  writeFileSync(join(config, "frostmail/app.json"), JSON.stringify({ lodge: `http://127.0.0.1:${port}` }));
  app = await launchApp(
    dir,
    { XDG_CONFIG_HOME: config, XDG_CACHE_HOME: join(dir, "app-cache"), FROSTMAIL_SOCKET: join(run, "none.sock") },
    procs,
  );
}, 90_000);

afterAll(async () => {
  await app?.stop();
  sink?.stop();
});

/** otherWindow waits for a window besides main and switches to it. */
async function otherWindow(s: Session, main: string): Promise<void> {
  const handle = await s.waitFor("a compose window", async () => (await s.windows()).find((h) => h !== main));
  await s.switchTo(handle);
  await s.waitFor("the compose toolbar", () => s.find('[role="toolbar"][aria-label="Compose"]'));
}

describe.skipIf(!LODGE)("the desktop app on a maild behind lodge", () => {
  it("lists, reads and replies through lodge", async () => {
    if (!app || !sink) throw new Error("the app did not start");
    const s = app.session;
    const main = await s.window();
    await s.waitFor("message rows", async () => (await s.findAll('[role="option"]')).length > 5, 30_000);
    expect(maildLog).toContain("lodge session started");

    const [first] = await s.findAll('[role="option"]');
    if (!first) throw new Error("no rows");
    await s.click(first);
    await s.waitFor("the reader header", async () => (await s.findAll('section[aria-label="Message"] header'))[0]);

    await s.click(await s.find('button[aria-label="Reply"]'));
    await otherWindow(s, main);
    const body = await s.find(".compose-body");
    await s.click(body);
    await s.type(body, "Replied through lodge.");
    await s.waitFor("Send to be enabled", async () =>
      s.execute<boolean>('return !document.querySelector("button[aria-label=Send]").disabled'),
    );
    await s.click(await s.find('button[aria-label="Send"]'));
    await s.switchTo(main);
    const sent = await s.waitFor("the reply at the SMTP sink", async () => sink?.messages()[0], 30_000);
    expect(sent).toContain("Replied through lodge.");
    expect(sent).toMatch(/^In-Reply-To: </m);
    writeFileSync(join(BUILD, "e2e-lodge.png"), Buffer.from(await s.screenshot(), "base64"));
  });
});
