// The change of mode in the Flatpak (docs/specs/settings-ui.md, Server pane,
// Behavior (container); ADR-0046, How duplicate engines are prevented): the
// Server pane writes the app's own choice, and the shell then stops this
// computer's maild before the app goes to lodge, and starts it when the app
// comes back, in one fixed order that never changes half-way. The shell runs
// `mailctl shutdown` and `frostmail --start-maild` as processes, so the two
// stand-ins below are those on the app's PATH; the maild that `frostmail
// --start-maild` starts is a real one on the fixture, and the test ends it.
// Windows skips this file: the change of mode is the Flatpak's.

import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import { type App, BUILD, bin, fixture, freePort, launch, tempDir, until, WINDOWS } from "./harness";
import { type Element, Key, type Session } from "./webdriver";

/** TREE is the sidebar's mailbox tree. */
const TREE = '[role="tree"][aria-label="Mailboxes"]';

/** NOTE_TO_LODGE is what saving lodge as the mode says (ServerPane.tsx). */
const NOTE_TO_LODGE =
  "This computer's maild stops. Its accounts and mail stay on this computer, not synced, until you switch back. Nothing moves to lodge.";

/** NOTE_TO_LOCAL_DATA is what saving this computer's maild says when it holds data. */
const NOTE_TO_LOCAL_DATA =
  "This computer's maild starts again with the accounts it had. It runs their rules, Send Later and reminders here. If lodge's maild has the same accounts, both run them.";

/** DID_NOT_STOP is the shell's words when this computer's maild does not stop. */
const DID_NOT_STOP = "This computer's maild did not stop. Frostmail still uses it.";

/** RECONNECTING begins the main window's text while it has no maild. */
const RECONNECTING = "Reconnecting to maild…";

let dir = "";
let data = "";
let socket = "";
let address = "";
let app: App | undefined;

/** ctlEnv is the environment the test's own mailctl calls get: this run's. */
function ctlEnv(): NodeJS.ProcessEnv {
  const env = { ...process.env, FROSTMAIL_SOCKET: socket, FROSTMAIL_DATA_DIR: data };
  return { ...env, FROSTMAIL_CACHE_DIR: join(dir, "cache") };
}

/** hello reports whether this computer's maild answers. */
function hello(): boolean {
  return spawnSync(bin("mailctl"), ["hello"], { env: ctlEnv(), stdio: "ignore", timeout: 20_000 }).status === 0;
}

/** storedLodge is the lodge address app.json holds, or undefined for this computer. */
function storedLodge(): string | undefined {
  try {
    const config = JSON.parse(readFileSync(join(dir, "config/frostmail/app.json"), "utf8")) as { lodge?: string };
    return config.lodge;
  } catch {
    return undefined;
  }
}

/** settingsWindow waits for the settings window and switches to it. */
async function settingsWindow(s: Session, main: string): Promise<string> {
  const others = async () => (await s.windows()).find((h) => h !== main);
  const handle = await s.waitFor("the settings window", others, 40_000);
  await s.switchTo(handle);
  return handle;
}

/** buttonWithText waits for the first button whose text is text. */
function buttonWithText(s: Session, text: string): Promise<Element> {
  return s.waitFor(`the ${text} button`, async () => {
    for (const el of await s.findAll("button")) {
      if ((await s.text(el)).trim() === text) return el;
    }
    return undefined;
  });
}

/** clickButton clicks a button by its text, such as the Server tab. */
async function clickButton(s: Session, text: string): Promise<void> {
  await s.click(await buttonWithText(s, text));
}

/**
 * chooseMode picks the Server pane's radio: 0 is this computer's maild and 1
 * is the maild behind lodge, in the pane's own order. It then waits until the
 * radio is checked, so a pane that reads the stored choice afterwards cannot
 * undo the click unseen.
 */
async function chooseMode(s: Session, which: 0 | 1): Promise<void> {
  const radio = (await s.findAll('input[name="server"]'))[which];
  if (!radio) throw new Error(`the Server pane has no radio ${which}`);
  await s.click(radio);
  const selector = 'input[name="server"]';
  const checked = () => s.execute<boolean>(`return document.querySelectorAll('${selector}')[${which}].checked;`);
  await s.waitFor("the radio to be checked", checked);
}

/** typeLodge types the lodge address, and waits until the field holds it. */
async function typeLodge(s: Session): Promise<void> {
  const field = await s.waitFor("the lodge field", () => s.find("#server-lodge"));
  await s.type(field, address);
  const value = () => s.execute<string>('return document.getElementById("server-lodge").value');
  await s.waitFor("the lodge field to hold the address", async () => (await value()) === address);
}

/** note returns the pane's role="note" text once it shows. */
async function note(s: Session): Promise<string> {
  return s.text(await s.waitFor("the note", () => s.find('[role="note"]')));
}

/** save clicks the pane's Save. The shell's change can take up to 35 s. */
async function save(s: Session): Promise<void> {
  await s.click(await s.find('form[aria-label="Server"] button[type="submit"]'));
}

/** accountNames are the accounts' own section titles in the mailbox tree. */
function accountNames(s: Session): Promise<string[]> {
  return s.execute<string[]>(
    `const tree = document.querySelector('${TREE}');
     if (!tree) return [];
     const titles = [...tree.querySelectorAll("button > span")].map((e) => e.innerText);
     return titles.filter((t) => t.includes("@"));`,
  );
}

/** reconnectText is the main window's reconnect text, when it shows one. */
async function reconnectText(s: Session): Promise<string | undefined> {
  const text = await s.execute<string>(
    `const el = [...document.querySelectorAll("span")].find((e) => e.innerText.startsWith("Reconnecting to maild"));
     return el ? el.innerText : "";`,
  );
  return text.length > 0 ? text : undefined;
}

/** endMaild ends the maild that `frostmail --start-maild` started, when it runs. */
async function endMaild(): Promise<void> {
  let pid = 0;
  try {
    pid = Number.parseInt(readFileSync(join(dir, "maild2.pid"), "utf8").trim(), 10);
  } catch {
    return;
  }
  if (!Number.isInteger(pid) || pid <= 0) return;
  const runs = () => {
    try {
      process.kill(pid, 0);
      return true;
    } catch {
      return false;
    }
  };
  try {
    process.kill(pid, "SIGTERM");
    await until("the started maild to end", () => !runs(), 10_000);
  } catch {
    if (runs()) process.kill(pid, "SIGKILL");
  }
}

describe.skipIf(WINDOWS)("the change of mode", () => {
  beforeAll(async () => {
    dir = tempDir("mode");
    data = fixture(dir, 3);
    socket = join(dir, "run/maild.sock");
    address = `http://127.0.0.1:${await freePort()}`;
    // The shell runs these two commands as processes. mailctl is the real one,
    // except a shutdown the test makes fail; frostmail is the launcher's
    // --start-maild, which starts a maild on this run's socket and data.
    const stand = join(dir, "bin");
    mkdirSync(stand, { recursive: true });
    writeFileSync(
      join(stand, "mailctl"),
      `#!/bin/sh
if [ "\${1:-}" = "shutdown" ] && [ -e "${dir}/stuck" ]; then
    echo "maild still holds the lock" >&2
    exit 1
fi
exec "${bin("mailctl")}" "$@"
`,
      { mode: 0o755 },
    );
    writeFileSync(
      join(stand, "frostmail"),
      `#!/bin/sh
if [ "\${1:-}" != "--start-maild" ]; then
    echo "frostmail: this stand-in takes only --start-maild" >&2
    exit 2
fi
if "${bin("mailctl")}" hello >/dev/null 2>&1; then
    exit 0
fi
# "$1" and "$2" are the inner shell's, so that the file holds maild's own pid.
# shellcheck disable=SC2016
setsid sh -c 'echo $$ >"$1"; exec "$2"' sh "${dir}/maild2.pid" "${bin("maild")}" </dev/null >"${dir}/maild2.log" 2>&1 &
for _ in $(seq 1 40); do
    sleep 0.25
    if "${bin("mailctl")}" hello >/dev/null 2>&1; then
        exit 0
    fi
done
echo "frostmail: maild did not start through the Flatpak portal (flatpak-spawn); see ${dir}/maild2.log" >&2
exit 1
`,
      { mode: 0o755 },
    );
    app = await launch(dir, data, {
      // The shell then changes the mode in its fixed order (paths::in_flatpak).
      FLATPAK_ID: "org.frostyard.Frostmail",
      XDG_CONFIG_HOME: join(dir, "config"),
      XDG_STATE_HOME: join(dir, "state"),
      // No session bus here, and the change needs only maild's answers: the
      // same file store the Flatpak's own smoke test gives maild.
      FROSTMAIL_SECRETS: "file",
      PATH: `${stand}:${BUILD}:${process.env.PATH ?? ""}`,
      FROSTMAIL_SOCKET: socket,
    });
  }, 120_000);

  afterAll(async () => {
    // What the started maild said, and the stored choice, for a failure.
    for (const name of ["maild2.log", "config/frostmail/app.json"]) {
      const file = join(dir, name);
      if (existsSync(file)) process.stderr.write(`--- ${name}\n${readFileSync(file, "utf8")}\n`);
    }
    await endMaild();
    await app?.stop();
  });

  it("stops and starts this computer's maild around the app's choice", async () => {
    if (!app) throw new Error("the app did not start");
    const s = app.session;
    const main = await s.window();

    // 1. The main window lists the account's mail; Ctrl+, opens the settings
    // window, and the Server tab is the pane.
    await s.waitFor("the mailbox tree", () => s.find(TREE), 40_000);
    const accounts = await accountNames(s);
    expect(accounts).toContain("test1@mailtest.test");
    await s.execute(`document.querySelector('${TREE}').focus()`);
    await s.keys(Key.Control, ",");
    const settings = await settingsWindow(s, main);
    await s.waitFor("the settings window's tabs", () => s.find("button[aria-pressed]"), 40_000);
    await clickButton(s, "Server");
    await s.waitFor("the Server pane", () => s.find('form[aria-label="Server"]'));

    // 2. To lodge: the note says what the change does, then the save runs.
    await chooseMode(s, 1);
    await typeLodge(s);
    expect(await note(s)).toBe(NOTE_TO_LODGE);
    await save(s);

    // 3. app.json names lodge, this computer's maild has stopped, and its data
    // stays; the main window has no maild and offers the Server pane.
    await until("app.json to name lodge", () => storedLodge() === address, 40_000);
    await until("this computer's maild to stop", () => !hello(), 40_000);
    expect(existsSync(join(data, "frostmail.db"))).toBe(true);
    await s.switchTo(main);
    const lost = await s.waitFor("the reconnect text", () => reconnectText(s), 40_000);
    expect(lost.startsWith(RECONNECTING)).toBe(true);
    const offered = await buttonWithText(s, "Server Settings…");
    expect(await s.visible(offered)).toBe(true);

    // 4. Back to this computer: the pane stays on Server without a maild, and
    // the note says what starting this computer's maild again does.
    await s.switchTo(settings);
    await s.waitFor("the stored lodge address", () => s.find("#server-lodge"), 40_000);
    await chooseMode(s, 0);
    expect(await note(s)).toBe(NOTE_TO_LOCAL_DATA);
    await save(s);

    // 5. app.json holds no lodge, this computer's maild answers again, and the
    // main window lists the same account. Each wait names what it waited
    // for, in order, so a failure says which step of the change did not
    // happen.
    const alerts = () => s.findAll('[role="alert"]');
    const saving = async () => (await s.findAll('[role="status"]')).length > 0;
    const ended = async () => storedLodge() === undefined || (await alerts()).length > 0;
    await s.waitFor("the save to finish", async () => !(await saving()) && (await ended()), 60_000);
    const [failed] = await alerts();
    if (failed) throw new Error(`the change to this computer failed: ${await s.text(failed)}`);
    await until("app.json to name this computer again", () => storedLodge() === undefined, 10_000);
    await until("this computer's maild to answer", hello, 40_000);
    await s.switchTo(main);
    await s.waitFor("the mailbox tree to return", () => s.find(TREE), 60_000);
    expect(await accountNames(s)).toEqual(accounts);

    // 6. A shutdown that fails writes the previous choice back: the mode stays
    // this computer's, and the pane says so.
    writeFileSync(join(dir, "stuck"), "");
    await s.switchTo(settings);
    await chooseMode(s, 1);
    await typeLodge(s);
    await save(s);
    const alert = await s.waitFor("the failure", () => s.find('[role="alert"]'), 60_000);
    expect(await s.text(alert)).toBe(DID_NOT_STOP);
    expect(storedLodge()).toBeUndefined();
    expect(hello()).toBe(true);
  }, 300_000);
});
