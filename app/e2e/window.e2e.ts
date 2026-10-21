// The window chrome and the keyboard map (docs/design/app.md, Window;
// docs/specs/ui.md, Keyboard map), driven as real key presses through the
// WebDriver actions endpoint. WebView2 has browser accelerator keys, such
// as Ctrl+R, Ctrl+F and Ctrl+N, that the app must take before the browser
// does; a marker in the page proves at the end that no key reloaded it.
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

/** button waits for a control with an accessible name. */
function button(s: Session, name: string): Promise<Element> {
  return s.waitFor(`the ${name} button`, async () => (await s.findAll(`button[aria-label="${name}"]`))[0]);
}

/** row waits for a message row by index. */
function row(s: Session, index: number): Promise<Element> {
  return s.waitFor(`message row ${index}`, async () => (await s.findAll('[role="option"]'))[index]);
}

describe("the window", () => {
  it("the keyboard map", async () => {
    if (!app) throw new Error("app did not start");
    const s = app.session;
    const main = await s.window();

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
    for (const name of ["Minimize", "Maximize", "Close"]) await button(s, name);

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

    // Minimize last: the window hides and stops taking keys.
    await s.click(await button(s, "Minimize"));
    await s.waitFor("the window to hide", () =>
      s.execute<boolean>('return document.visibilityState === "hidden" || document.hidden'),
    );
  });
});
