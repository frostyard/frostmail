// The window chrome and the keyboard map (docs/design/app.md, Window;
// docs/specs/ui.md, Keyboard map), driven as real key presses through the
// WebDriver actions endpoint. WebView2 has browser accelerator keys, such
// as Ctrl+R, Ctrl+F and Ctrl+N, that the app must take before the browser
// does; a marker in the page proves at the end that no key reloaded it.
// WebView2 keeps a minimized window's page visible, so the chrome test asks
// the window plugin whether the window is minimized.
import { afterAll, beforeAll, describe, expect, it } from "vitest";

import { type App, fixture, launch, tempDir, WINDOWS } from "./harness";
import { type Element, Key, type Session } from "./webdriver";

let app: App | undefined;

beforeAll(async () => {
  const dir = tempDir("window");
  app = await launch(dir, fixture(dir, 50));
  const s = app.session;
  await s.waitFor("message rows", async () => (await s.findAll('[role="option"]')).length > 5, 30_000);
  // Keys that reach the browser reload the page and lose this.
  await s.execute("window.__frostmailE2E = 1");
});

afterAll(async () => {
  await app?.stop();
});

/** rowSelected reports whether the row at index carries the list's selection. */
function rowSelected(s: Session, index: number): Promise<boolean> {
  return s.execute<boolean>(
    `const r = document.querySelectorAll('[role="option"]')[${index}];
     return !!r && r.getAttribute("aria-selected") === "true";`,
  );
}

/** selectedId returns the message id of the selected row. */
function selectedId(s: Session): Promise<string> {
  return s.execute<string>(
    `const r = document.querySelector('[role="option"][aria-selected="true"]');
     return r ? r.dataset.messageId : "";`,
  );
}

/** unread reports whether the list draws the unread dot in the message's row. */
function unread(s: Session, id: string): Promise<boolean> {
  return s.execute<boolean>(
    `const r = document.querySelector('[role="option"][data-message-id="${id}"]');
     return !!(r && r.querySelector('[aria-label="Unread"]'));`,
  );
}

/** button waits for the first visible control with an accessible name. */
function button(s: Session, name: string): Promise<Element> {
  return s.waitFor(`the ${name} button`, async () => {
    for (const el of await s.findAll(`button[aria-label="${name}"]`)) {
      if (await s.visible(el)) return el;
    }
    return undefined;
  });
}

/** CaptionBox is one window button's box, in the page's pixels. */
interface CaptionBox {
  name: string;
  width: number;
  height: number;
  right: number;
}

/** CAPTIONS measures the three window buttons against the strip and the window. */
const CAPTIONS = `const box = (el) => { const r = el.getBoundingClientRect(); return { name: el.getAttribute("aria-label"), width: r.width, height: r.height, right: r.right }; };
  const buttons = ["Minimize", "Maximize", "Restore", "Close"]
    .map((name) => document.querySelector('button[aria-label="' + name + '"]'))
    .filter((el) => el !== null)
    .map(box);
  const strip = document.querySelector('[role="toolbar"][aria-label="Toolbar"]');
  return { buttons: buttons, strip: strip ? strip.clientHeight : 0, window: window.innerWidth };`;

/** MINIMIZED asks Tauri's window plugin whether the main window is minimized. */
const MINIMIZED = `const done = arguments[arguments.length - 1];
  window.__TAURI_INTERNALS__.invoke("plugin:window|is_minimized", { label: "main" }).then((v) => done(v), (e) => done("error: " + e));`;

/** row waits for a message row by index. */
function row(s: Session, index: number): Promise<Element> {
  return s.waitFor(`message row ${index}`, async () => (await s.findAll('[role="option"]'))[index]);
}

/** controlsInside fails, naming the bad box, unless each window control is in the viewport. */
async function controlsInside(s: Session): Promise<void> {
  const outside = await s.execute<string>(
    `const bad = [...document.querySelectorAll('button[aria-label="Minimize"], button[aria-label="Maximize"], button[aria-label="Restore"], button[aria-label="Close"]')]
       .map((el) => ({ name: el.getAttribute("aria-label"), right: el.getBoundingClientRect().right }))
       .filter((b) => b.right > window.innerWidth);
     return bad.map((b) => b.name + " at right " + Math.round(b.right) + " > " + window.innerWidth).join("; ");`,
  );
  expect(outside, "window controls outside the window").toBe("");
}

describe("the window", () => {
  it("the keyboard map", async () => {
    if (!app) throw new Error("app did not start");
    const s = app.session;
    const main = await s.window();
    // The window controls stay in the window at this width; Linux checks
    // the same boxes at its 1200-pixel window.
    await controlsInside(s);
    // The window plugin answers through the page's internals on Linux too.
    expect(await s.executeAsync<boolean>(MINIMIZED)).toBe(false);

    await s.click(await row(s, 0));
    await s.waitFor("the first row to be selected", () => rowSelected(s, 0));
    await s.keys(Key.ArrowDown);
    await s.waitFor("the second row to be selected", () => rowSelected(s, 1));
    expect(await rowSelected(s, 0)).toBe(false);

    // The reader marks what it shows as read; wait for that, then toggle
    // the selected message unread and read again with Ctrl+Shift+U.
    const id = await selectedId(s);
    await s.waitFor("the reader to mark the message read", async () => !(await unread(s, id)));
    await s.keys(Key.Control, Key.Shift, "u");
    await s.waitFor("the message to become unread", () => unread(s, id));
    await s.keys(Key.Control, Key.Shift, "u");
    await s.waitFor("the message to become read", async () => !(await unread(s, id)));

    await s.keys(Key.Control, Key.Alt, "s");
    await s.waitFor("the sidebar to hide", async () => (await s.findAll('[role="tree"]')).length === 0);
    await s.keys(Key.Control, Key.Alt, "s");
    await s.waitFor("the sidebar to return", async () => (await s.findAll('[role="tree"]')).length > 0);

    await s.keys(Key.Control, "f");
    await s.waitFor("the search field to take the focus", () =>
      s.execute<boolean>('return document.activeElement === document.querySelector("input[aria-label=Search]")'),
    );
    await s.keys(Key.Escape);

    // Escape leaves the keys in the search field, where Ctrl+R is not
    // bound; a click puts the list in charge, and there Ctrl+R replies.
    await s.click(await row(s, 1));
    await s.keys(Key.Control, "r");
    const compose = await s.waitFor("a compose window", async () => (await s.windows()).find((h) => h !== main));
    await s.switchTo(compose);
    await s.waitFor("the compose toolbar", () => s.find('[role="toolbar"][aria-label="Compose"]'));
    // Closing an untouched reply deletes its draft and closes its window.
    await s.click(await button(s, "Close"));
    await s.waitFor("the compose window to close", async () => (await s.windows()).length === 1);
    await s.switchTo(main);

    expect(await s.execute<number>("return window.__frostmailE2E")).toBe(1);
  });

  // Xvfb in make ui-e2e has no window manager, so a window cannot maximize
  // there; the chrome is checked in WebView2 on Windows only.
  it.skipIf(!WINDOWS)("the window chrome", async () => {
    if (!app) throw new Error("app did not start");
    const s = app.session;
    // A narrow window must not push the undecorated window's controls out of
    // its right edge, where no pointer can reach them.
    await controlsInside(s);
    for (const name of ["Minimize", "Maximize", "Close"]) await button(s, name);

    // Windows draws Windows' caption buttons: each 46 wide, as high as the
    // strip (its 52px less the 1px border below it), and Close reaching the
    // window's right edge, with no padding after it (docs/specs/ui.md,
    // Layout: Toolbar; ADR-0038).
    const caption = await s.execute<{ buttons: CaptionBox[]; strip: number; window: number }>(CAPTIONS);
    expect(caption.buttons.map((b) => b.name)).toEqual(["Minimize", "Maximize", "Close"]);
    for (const box of caption.buttons) {
      expect(Math.abs(box.width - 46), `${box.name} is 46 wide`).toBeLessThanOrEqual(1);
      expect(Math.abs(box.height - caption.strip), `${box.name} fills the strip`).toBeLessThanOrEqual(1);
    }
    const close = caption.buttons[2];
    if (!close) throw new Error("the Close button did not measure");
    expect(Math.abs(close.right - caption.window), "Close reaches the window's right edge").toBeLessThanOrEqual(1);

    const before = await s.execute<number>("return window.outerWidth");
    await s.click(await button(s, "Maximize"));
    await button(s, "Restore");
    const widened = await s.waitFor("the window to maximize", async () => {
      const now = await s.execute<number>("return window.outerWidth");
      const avail = await s.execute<number>("return screen.availWidth");
      return now > before || now >= avail;
    });
    expect(widened).toBe(true);

    await s.click(await button(s, "Restore"));
    await button(s, "Maximize");

    // Minimize last: the window leaves the screen, so ask the window plugin.
    await s.click(await button(s, "Minimize"));
    const state = await s.waitFor("the window to minimize", async () => {
      const v = await s.executeAsync<boolean | string>(MINIMIZED);
      return typeof v === "string" || v ? v : undefined;
    });
    if (typeof state === "string" && state.startsWith("error:")) throw new Error(state);
    expect(state).toBe(true);
  });
});
