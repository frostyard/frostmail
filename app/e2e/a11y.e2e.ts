// Accessibility (docs/specs/ui.md, Rules: "Every interactive element is
// reachable by keyboard and has an accessible name"): axe-core audits the
// built app's views on the showcase's made-up mail, and the test fails on
// each serious or critical violation. The views are the main window, the
// reader, compose, settings, calendar, people and tasks (frostmail#200).
//
// axe-core runs in the page: the harness reads its bundle from
// node_modules and injects it through WebDriver, as the other app tests run
// their scripts. Moderate and minor findings are printed, not failed.
import { mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { basename, join } from "node:path";

import { afterEach, describe, expect, it, onTestFinished } from "vitest";
import { endLaunch } from "./endLaunch";
import { type App, fixture, launch, ROOT, tempDir, WINDOWS } from "./harness";
import type { Session } from "./webdriver";

const SHOWCASE = join(ROOT, "app/e2e/showcase");
const AXE = readFileSync(createRequire(import.meta.url).resolve("axe-core/axe.min.js"), "utf8");

/** FAIL is the impacts that fail the test. */
const FAIL = new Set(["serious", "critical"]);

/**
 * REPORT_ONLY names the rules whose findings are printed but do not fail.
 * color-contrast: the text colors are the spec's tokens (docs/specs/ui.md,
 * Tokens), which lodge-ui provides to the whole suite; `--text-secondary`
 * and `--text-tertiary` follow the macOS label colors and are below 4.5:1 on
 * the window's grays. A change of those tokens is a change of the spec and
 * of lodge-ui, not of this app's code (frostmail#200).
 */
const REPORT_ONLY = new Set(["color-contrast"]);

let app: App | undefined;

let mainHandle: string | undefined;

afterEach(async () => {
  const launched = app;
  app = undefined;
  if (!launched) return;
  const s = launched.session;
  // A launch must end before the next one: the app is single-instance (over
  // D-Bus, as in CI), and a lingering app would take the next launch. Close
  // the windows a test opened (compose, settings, a reminder) first. A window
  // that goes after it was listed, or a WebDriver call that hangs, must not
  // skip the steps after it, so endLaunch bounds each step and still ends the
  // app. These tests check the views, not how the app ends (firstrun.e2e.ts
  // does); an app that outlives its session is ended, and the log says so.
  await endLaunch(
    {
      closeOthers: async () => {
        for (const handle of await s.windows()) {
          if (handle === mainHandle) continue;
          await s.switchTo(handle);
          await s.closeWindow();
        }
      },
      stop: () => launched.stop(),
      running: () => launched.running(),
      end: () => endApp(launched.cache),
    },
    (line) => process.stderr.write(`${line}\n`),
  );
});

/**
 * endApp sends SIGTERM to each frostmail-app process launched with cache as
 * its FROSTMAIL_CACHE_DIR, and returns their pids. Linux only (/proc); on
 * Windows the harness starts the app and ends it itself.
 */
function endApp(cache: string): string[] {
  const ended: string[] = [];
  let entries: string[] = [];
  try {
    entries = readdirSync("/proc");
  } catch {
    return ended;
  }
  for (const pid of entries.filter((e) => /^\d+$/.test(e))) {
    try {
      const cmd = readFileSync(`/proc/${pid}/cmdline`, "utf8").split("\0")[0] ?? "";
      if (basename(cmd) !== "frostmail-app") continue;
      if (!readFileSync(`/proc/${pid}/environ`, "utf8").split("\0").includes(`FROSTMAIL_CACHE_DIR=${cache}`)) continue;
      process.kill(Number(pid), "SIGTERM");
      ended.push(pid);
    } catch {
      // The process ended during the scan, or it is another user's.
    }
  }
  return ended;
}

/** Finding is one axe violation with the elements it names. */
interface Finding {
  id: string;
  impact: string | null;
  help: string;
  nodes: { target: string; html: string; summary: string }[];
}

/**
 * audit runs axe on the current window's document and returns its
 * violations. The reader's message frame is not audited (iframes: false):
 * it holds the sender's HTML, not the app's UI, and it runs no script
 * (ADR-0005), so axe cannot run inside it. The frame element itself, its
 * title for one, is part of the app's document and is audited.
 */
async function audit(s: Session): Promise<{ rules: number; violations: Finding[] }> {
  await s.execute(`if (!window.axe) { ${AXE}\n }`);
  const result = await s.executeAsync<{ error?: string; rules?: number; violations?: Finding[] }>(
    `const done = arguments[arguments.length - 1];
     window.axe
       .run(document, { iframes: false, resultTypes: ["violations"] })
       .then((r) => done({
         rules: r.passes.length + r.violations.length + r.incomplete.length,
         violations: r.violations.map((v) => ({
           id: v.id,
           impact: v.impact,
           help: v.help,
           nodes: v.nodes.map((n) => ({
             target: n.target.join(" "),
             html: n.html.slice(0, 200),
             summary: n.failureSummary || "",
           })),
         })),
       }))
       .catch((e) => done({ error: String(e) }));`,
  );
  if (result.error) throw new Error(`axe: ${result.error}`);
  return { rules: result.rules ?? 0, violations: result.violations ?? [] };
}

/** fails reports whether a finding fails the test. */
const fails = (f: Finding) => FAIL.has(f.impact ?? "") && !REPORT_ONLY.has(f.id);

/** check audits the current window as view and fails on serious and critical violations. */
async function check(s: Session, view: string): Promise<void> {
  const { rules, violations: found } = await audit(s);
  const failingCount = found.filter(fails).length;
  process.stderr.write(`a11y: ${view}: ${rules} rules applied, ${found.length} violated, ${failingCount} failing\n`);
  // An audit that applied no rule checked nothing.
  expect(rules, `${view}: axe applied no rule`).toBeGreaterThan(0);
  const show = (f: Finding) =>
    `${view}: ${f.impact} ${f.id} (${f.help})\n${f.nodes.map((n) => `  ${n.target}\n    ${n.html}\n    ${n.summary.replaceAll("\n", "\n    ")}`).join("\n")}`;
  const failing = found.filter(fails);
  for (const f of found.filter((f) => !fails(f))) {
    process.stderr.write(`${view}: ${f.impact} ${f.id} (${f.help}), ${f.nodes.length} element(s)\n`);
  }
  expect.soft(failing.map(show), `${view} has serious or critical accessibility violations`).toEqual([]);
}

/** start launches the app on the showcase and waits for the inboxes. */
async function start(prefix: string, env: Record<string, string> = {}): Promise<Session> {
  const dir = tempDir(prefix);
  app = await launch(dir, fixture(dir, 0, { showcase: SHOWCASE }), { FROSTMAIL_SYNC: "off", ...env });
  const s = app.session;
  mainHandle = await s.window();
  await s.waitFor("message rows", async () => (await s.findAll('[role="option"]')).length >= 10, 30_000);
  // WebKit keeps localStorage across launches: audit the default layout
  // (no To-Do bar), not the last run's.
  await s.execute(`localStorage.removeItem("frostmail.ui"); location.reload();`);
  await s.waitFor(
    "message rows after a reload",
    () =>
      s.execute<boolean>(
        `return document.querySelectorAll('[role="option"]').length >= 10 && !document.querySelector("aside");`,
      ),
    30_000,
  );
  return s;
}

/** todoBar turns the persisted To-Do bar setting on and reloads. */
async function todoBar(s: Session): Promise<void> {
  await s.execute(
    `const v = JSON.parse(localStorage.getItem("frostmail.ui") || '{"state":{},"version":0}');
     v.state.todoBar = true;
     localStorage.setItem("frostmail.ui", JSON.stringify(v));
     location.reload();`,
  );
  await s.waitFor("the To-Do bar's tasks", () =>
    s.execute<boolean>(
      `return document.querySelectorAll('[role="option"]').length >= 10 &&
         document.querySelectorAll('aside[aria-label="To-Do Bar"] li').length >= 3;`,
    ),
  );
}

/** open selects the row whose text holds row and waits for the reader to show body. */
async function open(s: Session, row: string, body: string): Promise<void> {
  let found = false;
  for (const r of await s.findAll('[role="option"]')) {
    if ((await s.text(r)).includes(row)) {
      await s.click(r);
      found = true;
      break;
    }
  }
  if (!found) throw new Error(`no row says ${row}`);
  await s.waitFor(`the reader to show ${body}`, () =>
    s.execute<boolean>(
      `const pane = document.querySelector('section[aria-label="Message"]');
       if (!pane) return false;
       const docs = [...pane.querySelectorAll("iframe")].map((f) => f.contentDocument);
       const text = pane.innerText + docs.map((d) => (d && d.body ? d.body.innerText : "")).join("");
       return text.includes(arguments[0]);`,
      body,
    ),
  );
}

/** module shows a module from the Modules toolbar. */
async function module(s: Session, name: string): Promise<void> {
  await s.click(await s.find(`[role="toolbar"][aria-label="Modules"] button[aria-label="${name}"]`));
}

/** newWindow waits for a window not in known that shows selector, and switches to it. */
async function newWindow(s: Session, known: string[], selector: string): Promise<string> {
  return s.waitFor(`a new window with ${selector}`, async () => {
    for (const handle of await s.windows()) {
      if (known.includes(handle)) continue;
      await s.switchTo(handle);
      if (await s.find(selector).catch(() => null)) return handle;
    }
    return undefined;
  });
}

describe("accessibility", () => {
  it("fails on a control with no accessible name", async () => {
    // The audit must be able to fail: a button with no name is a critical
    // violation (button-name) that the check counts as failing.
    const s = await start("a11y-canary");
    await s.execute(`const b = document.createElement("button"); b.id = "a11y-canary"; document.body.append(b);`);
    const { violations } = await audit(s);
    const named = violations.filter((f) => f.id === "button-name" && fails(f));
    expect(named.flatMap((f) => f.nodes.map((n) => n.target))).toContain("#a11y-canary");
  });

  it.skipIf(WINDOWS)("stops animations when the system asks for reduced motion", async () => {
    // WebKitGTK reports prefers-reduced-motion: reduce when GTK's animations
    // are off (gtk-enable-animations in the user's GTK settings).
    const config = tempDir("a11y-motion-config");
    mkdirSync(join(config, "gtk-3.0"), { recursive: true });
    writeFileSync(join(config, "gtk-3.0/settings.ini"), "[Settings]\ngtk-enable-animations=false\n");
    onTestFinished(() => rmSync(config, { recursive: true, force: true }));
    const s = await start("a11y-motion", { XDG_CONFIG_HOME: config });
    const seen = await s.execute<{ reduce: boolean; seconds: number; count: string }>(
      `const el = document.createElement("span");
       el.className = "animate-spin";
       document.body.append(el);
       const style = getComputedStyle(el);
       const seen = {
         reduce: matchMedia("(prefers-reduced-motion: reduce)").matches,
         seconds: Number.parseFloat(style.animationDuration),
         count: style.animationIterationCount,
       };
       el.remove();
       return seen;`,
    );
    // Without the rule, animate-spin runs for 1 s, without end.
    expect(seen.reduce).toBe(true);
    expect(seen.seconds).toBeLessThan(0.001);
    expect(seen.count).toBe("1");
  });

  it("has no serious or critical violations in the main window, the reader, compose and settings", async () => {
    const s = await start("a11y-mail");
    const main = await s.window();
    await check(s, "main window");

    await open(s, "Lisbon", "Casa do Rio");
    await check(s, "reader");

    await open(s, "Invitation: Release planning", "has invited you");
    await s.waitFor("the invitation card", () =>
      s.execute<boolean>(`return !!document.querySelector('section[aria-label="Invitation"] [role="group"]');`),
    );
    await check(s, "reader with an invitation");

    await todoBar(s);
    await check(s, "main window with the To-Do bar");

    await s.click(await s.find('button[aria-label="New Message"]'));
    const compose = await newWindow(s, [main], '[role="toolbar"][aria-label="Compose"]');
    await s.waitFor("the compose body", () => s.find(".compose-body"));
    await check(s, "compose");

    await s.switchTo(main);
    await s.click(await s.find('button[aria-label="Settings"]'));
    await newWindow(s, [main, compose], "#account-email");
    await check(s, "settings");
  });

  it("has no serious or critical violations in calendar, people and tasks", async () => {
    const s = await start("a11y-modules");

    await module(s, "Calendar");
    for (const view of ["Week", "Month", "Day"]) {
      for (const radio of await s.findAll('[role="radiogroup"][aria-label="View"] [role="radio"]')) {
        if ((await s.text(radio)) === view) await s.click(radio);
      }
      await s.waitFor(`the ${view} view`, () =>
        s.execute<boolean>(
          `const r = [...document.querySelectorAll('[role="radiogroup"][aria-label="View"] [role="radio"]')]
             .find((e) => e.textContent === arguments[0]);
           return !!r && r.getAttribute("aria-checked") === "true";`,
          view,
        ),
      );
      if (view === "Week") {
        await s.waitFor("the standups", () =>
          s.execute<boolean>(`return document.querySelectorAll('button[aria-label^="Standup"]').length > 0;`),
        );
        await s.click(await s.find('button[aria-label^="Design review,"]'));
        await s.waitFor("the event pane", () =>
          s.execute<boolean>(
            `const h = document.querySelector("h2"); return !!h && h.textContent === "Design review";`,
          ),
        );
      }
      await check(s, `calendar, ${view}`);
    }

    await module(s, "People");
    const contacts = '[role="listbox"][aria-label="Contacts"] [role="option"]';
    await s.waitFor("the people list", async () => (await s.findAll(contacts)).length >= 5);
    for (const r of await s.findAll(contacts)) {
      if ((await s.text(r)).includes("Maria Lopez")) {
        await s.click(r);
        break;
      }
    }
    await s.waitFor("Maria Lopez's card", () =>
      s.execute<boolean>(`const h = document.querySelector("h2"); return !!h && h.textContent === "Maria Lopez";`),
    );
    await check(s, "people");

    await module(s, "Tasks");
    for (const row of await s.findAll('[role="tree"][aria-label="Task Lists"] [role="treeitem"]')) {
      if ((await s.text(row)).startsWith("All Tasks")) {
        await s.click(row);
        break;
      }
    }
    const tasks = '[role="listbox"][aria-label="All Tasks"] [role="option"]';
    await s.waitFor("the tasks", async () => (await s.findAll(tasks)).length >= 3);
    for (const r of await s.findAll(tasks)) {
      if ((await s.text(r)).includes("Book the Lisbon venue")) {
        await s.click(r);
        break;
      }
    }
    await s.waitFor("the task pane", () =>
      s.execute<boolean>(
        `const t = document.querySelector('input[aria-label="Title"]'); return !!t && t.value === "Book the Lisbon venue";`,
      ),
    );
    await check(s, "tasks");
  });
});
