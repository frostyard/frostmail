// The README's screenshots (make screenshots): the built app on the
// showcase's made-up mail (app/e2e/showcase, built by tools/uifixture
// -showcase), in light and dark. Skipped unless FROSTMAIL_SCREENSHOTS names
// the directory, relative to the repository, that the PNGs go to.
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

import { afterEach, describe, it } from "vitest";

import { type App, fixture, launch, ROOT, tempDir } from "./harness";
import type { Session } from "./webdriver";

const out = process.env.FROSTMAIL_SCREENSHOTS;
const SHOWCASE = join(ROOT, "app/e2e/showcase");

let app: App | undefined;

afterEach(async () => {
  await app?.stop();
  app = undefined;
});

/** start launches the app on the showcase and waits for the inboxes. */
async function start(prefix: string, env: Record<string, string> = {}): Promise<Session> {
  const dir = tempDir(prefix);
  app = await launch(dir, fixture(dir, 0, { showcase: SHOWCASE }), { FROSTMAIL_SYNC: "off", ...env });
  const s = app.session;
  await s.waitFor("message rows", async () => (await s.findAll('[role="option"]')).length >= 10, 30_000);
  return s;
}

async function save(s: Session, name: string): Promise<void> {
  const dir = join(ROOT, out ?? "");
  mkdirSync(dir, { recursive: true });
  writeFileSync(join(dir, name), Buffer.from(await s.screenshot(), "base64"));
}

/** open selects the row whose text holds row and waits for the reader to show body, every frame and image loaded. */
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
       return text.includes(arguments[0]) &&
         docs.every((d) => d && d.body && [...d.images].every((i) => i.complete && i.naturalWidth > 0));`,
      body,
    ),
  );
}

/** people shows the People module with the person named selected. */
async function people(s: Session, name: string): Promise<void> {
  await s.click(await s.find('[role="toolbar"][aria-label="Modules"] button[aria-label="People"]'));
  const rows = '[role="listbox"][aria-label="Contacts"] [role="option"]';
  await s.waitFor("the people list", async () => (await s.findAll(rows)).length >= 5);
  for (const r of await s.findAll(rows)) {
    if ((await s.text(r)).includes(name)) {
      await s.click(r);
      break;
    }
  }
  await s.waitFor(`${name}'s card`, () =>
    s.execute<boolean>(
      `const h = document.querySelector("h2");
       return !!h && h.textContent === arguments[0] && document.body.innerText.includes("RECENT MAIL");`,
      name,
    ),
  );
}

/** calendar shows the Calendar module in a view, with the occurrence titled select selected. */
async function calendar(s: Session, view: "Day" | "Week" | "Month", select?: string): Promise<void> {
  await s.click(await s.find('[role="toolbar"][aria-label="Modules"] button[aria-label="Calendar"]'));
  for (const radio of await s.findAll('[role="radiogroup"][aria-label="View"] [role="radio"]')) {
    if ((await s.text(radio)) === view) await s.click(radio);
  }
  await s.waitFor("the standups", () =>
    s.execute<boolean>(`return document.querySelectorAll('button[aria-label^="Standup"]').length > 0;`),
  );
  if (!select) return;
  await s.click(await s.find(`button[aria-label^="${select},"]`));
  await s.waitFor(`${select} in the event pane`, () =>
    s.execute<boolean>(`const h = document.querySelector("h2"); return !!h && h.textContent === arguments[0];`, select),
  );
}

/** newWindow waits for a window not in known that shows selector (another,
 *  such as the reminder window, may open meanwhile), and switches to it. */
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

describe.skipIf(!out)("README screenshots", () => {
  it("shows the main window, search, compose and settings in light", async () => {
    const s = await start("shots-light");
    const main = await s.window();
    await open(s, "Lisbon", "Casa do Rio");
    await save(s, "main.png");

    const search = await s.find('input[aria-label="Search"]');
    await s.click(search);
    await s.type(search, "aurora\n");
    await s.waitFor("the search results", async () => (await s.findAll('[role="option"]')).length < 10);
    await open(s, "load test", "no errors");
    await save(s, "search.png");

    await s.click(await s.find('button[aria-label="Reply"]'));
    const compose = await newWindow(s, [main], '[role="toolbar"][aria-label="Compose"]');
    const reply = "Great news, thanks David! I'll mark it done on the board.";
    const body = await s.find(".compose-body");
    await s.click(body);
    // The click lands mid-quote; the reply goes on the empty first line.
    await s.execute(
      `const line = document.querySelector(".compose-body p");
       const range = document.createRange();
       range.setStart(line, 0);
       range.collapse(true);
       getSelection().removeAllRanges();
       getSelection().addRange(range);`,
    );
    await s.type(body, reply);
    await s.waitFor("the typed reply", async () => (await s.text(body)).includes(reply));
    await save(s, "compose.png");

    await s.switchTo(main);
    await s.click(await s.find('button[aria-label="Settings"]'));
    await newWindow(s, [main, compose], "#account-email");
    await save(s, "settings.png");
  });

  it("shows the main window in dark", async () => {
    const s = await start("shots-dark", { GTK_THEME: "Adwaita:dark" });
    await open(s, "Aurora launch checklist", "no errors");
    await save(s, "main-dark.png");
    await people(s, "Maria Lopez");
    await save(s, "people-dark.png");
  });

  it("shows People in light", async () => {
    const s = await start("shots-people");
    await people(s, "Maria Lopez");
    await save(s, "people.png");
  });

  it("shows Calendar in light and dark", async () => {
    const s = await start("shots-calendar");
    await calendar(s, "Week", "Design review");
    await save(s, "calendar.png");
    await calendar(s, "Month");
    await save(s, "calendar-month.png");
  });

  it("shows Calendar in dark", async () => {
    const s = await start("shots-calendar-dark", { GTK_THEME: "Adwaita:dark" });
    await calendar(s, "Week", "Design review");
    await save(s, "calendar-dark.png");
  });
});
