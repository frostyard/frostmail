// First run (docs/specs/ui.md, First run), on a maild data directory with no
// account: the main window shows the welcome and opens the settings window on
// Accounts by itself. The settings window belongs to the main window, so a
// close of the main window closes it too: no window is left to keep the app,
// and with it maild, running alone.
import { mkdirSync } from "node:fs";
import { join } from "node:path";

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import { type App, launch, tempDir, until } from "./harness";
import type { Element, Session } from "./webdriver";

let app: App | undefined;

beforeAll(async () => {
  const dir = tempDir("firstrun");
  // No fixture: maild starts on an empty store, which has no account.
  const data = join(dir, "data");
  mkdirSync(data);
  app = await launch(dir, data);
});

afterAll(async () => {
  await app?.stop();
});

/** button waits for the first visible control with an accessible name. */
function button(s: Session, name: string): Promise<Element> {
  return s.waitFor(`the ${name} button`, async () => {
    for (const el of await s.findAll(`button[aria-label="${name}"]`)) {
      if (await s.visible(el)) return el;
    }
    return undefined;
  });
}

/** mainWindow waits for the main window: the handle whose page holds the
 * welcome, which the settings window does not. The main window opens the
 * settings window on an accountless store, so 2 handles are there by then. */
function mainWindow(s: Session): Promise<string> {
  return s.waitFor(
    "the main window with the welcome and the settings window",
    async () => {
      const handles = await s.windows();
      if (handles.length < 2) return undefined;
      for (const handle of handles) {
        try {
          await s.switchTo(handle);
          if ((await s.findAll('section[aria-label="Welcome"]')).length > 0) return handle;
        } catch {
          // A window that closed under the loop is not the main window.
        }
      }
      return undefined;
    },
    30_000,
  );
}

describe("the first run", () => {
  it("closes the settings window with the main window", async () => {
    if (!app) throw new Error("app did not start");
    const launched = app;
    const s = launched.session;
    const main = await mainWindow(s);
    await s.switchTo(main);
    // The process check below must find this launch's app, or it would report
    // an end that never happened.
    expect(launched.running()).toBe(true);

    const close = await button(s, "Close");
    try {
      await s.click(close);
    } catch {
      // The window goes while the click is answered, which can lose the
      // reply; the app process, not the driver, says whether it ended.
    }

    // The settings window goes with it: with no window left, the app and its
    // maild end.
    await until("the app to end", () => !launched.running(), 20_000);
    expect(launched.running()).toBe(false);
  });
});
