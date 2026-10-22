// The list's filter tabs and Sort must fit without clipping or overlap,
// both in the default 360px column and in a narrow single-pane window.
import { afterAll, beforeAll, describe, expect, it } from "vitest";

import { type App, fixture, launch, tempDir, WINDOWS } from "./harness";
import { type Session, WebDriverError } from "./webdriver";

let app: App | undefined;
let cannotResize = "";

beforeAll(async () => {
  const dir = tempDir("narrow");
  app = await launch(dir, fixture(dir, 50));
  await app.session.waitFor("message rows", async () => (await app?.session.findAll('[role="option"]'))?.length);
});

afterAll(async () => {
  await app?.stop();
});

async function layout(s: Session) {
  await s.executeAsync(`const done = arguments[arguments.length - 1];
    document.fonts.ready.then(() => requestAnimationFrame(() => requestAnimationFrame(() => done(true))))`);
}

async function checkButtons(s: Session) {
  await layout(s);
  const result = await s.execute<{ names: string[]; errors: string[]; rowWidth: number }>(`
    const bar = document.querySelector('[role="toolbar"][aria-label="Filter messages"]');
    if (!bar) throw new Error("no filter bar");
    const row = bar.parentElement.parentElement;
    const buttons = [...bar.querySelectorAll(':scope > button'), ...row.querySelectorAll(':scope > button')];
    const errors = [];
    const epsilon = 0.5;
    const name = el => el.getAttribute('aria-label') || el.textContent;
    const inside = (r, box) => r.left >= box.left - epsilon && r.right <= box.right + epsilon &&
      r.top >= box.top - epsilon && r.bottom <= box.bottom + epsilon;
    for (const button of buttons) {
      const rect = button.getBoundingClientRect();
      if (!inside(rect, { left: 0, top: 0, right: innerWidth, bottom: innerHeight }))
        errors.push(name(button) + " outside the viewport");
      if (!inside(rect, row.getBoundingClientRect())) errors.push(name(button) + " outside the row");
      for (let parent = button.parentElement; parent; parent = parent.parentElement) {
        const p = parent.getBoundingClientRect();
        const visible = { left: p.left + parent.clientLeft, top: p.top + parent.clientTop,
          right: p.left + parent.clientLeft + parent.clientWidth,
          bottom: p.top + parent.clientTop + parent.clientHeight };
        if (!inside(rect, visible)) errors.push(name(button) + " outside " + parent.tagName + "." + parent.className);
      }
    }
    for (let i = 0; i < buttons.length; i++) {
      for (let j = i + 1; j < buttons.length; j++) {
        const a = buttons[i].getBoundingClientRect();
        const b = buttons[j].getBoundingClientRect();
        if (Math.min(a.right, b.right) - Math.max(a.left, b.left) > epsilon &&
            Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top) > epsilon)
          errors.push(name(buttons[i]) + " intersects " + name(buttons[j]));
      }
    }
    return { names: buttons.map(name), errors, rowWidth: row.getBoundingClientRect().width };
  `);
  expect(result.names).toContain("All");
  expect(result.names).toContain("More filters");
  expect(result.names.some((name) => name.startsWith("Sort by "))).toBe(true);
  expect(result.errors, result.errors.join("; ")).toEqual([]);
  return result;
}

describe("the filter row fits", () => {
  it("fits the default window's 360px list column", async () => {
    if (!app) throw new Error("app did not start");
    const result = await checkButtons(app.session);
    expect(result.rowWidth).toBe(360);
  });

  for (const width of [390, 360]) {
    it(`fits a ${width}px window`, async (context) => {
      if (!app) throw new Error("app did not start");
      const s = app.session;
      if (cannotResize) context.skip(cannotResize);
      try {
        const rect = await s.setWindowRect(width, 800);
        if (!WINDOWS && rect.width !== width) {
          cannotResize = `WebKitWebDriver kept a ${rect.width}px window instead of ${width}px. Only the default window is checked.`;
          console.warn(cannotResize);
          context.skip(cannotResize);
        }
      } catch (error) {
        if (
          !WINDOWS &&
          error instanceof WebDriverError &&
          ["unsupported operation", "unknown command", "unknown method"].includes(error.code)
        ) {
          cannotResize = `WebKitWebDriver cannot resize the app window: ${error.message}. Only the default window is checked.`;
          console.warn(cannotResize);
          context.skip(cannotResize);
        }
        throw error;
      }
      await s.waitFor(`${width}px viewport`, () => s.execute<boolean>(`return innerWidth === ${width}`));
      const result = await checkButtons(s);
      expect(result.rowWidth).toBe(width);

      // A chosen hidden filter gives More a longer label, which must fit too.
      await s.click(await s.find('button[aria-label="More filters"]'));
      const attachments = await s.waitFor("Attachments in More", async () => {
        for (const item of await s.findAll('[role="menuitemcheckbox"]')) {
          if ((await s.text(item)) === "Attachments") return item;
        }
        return undefined;
      });
      await s.click(attachments);
      await s.waitFor("the chosen More label", () =>
        s.execute<boolean>(`
        const more = document.querySelector('button[aria-label="More filters"]');
        return more.textContent === 'Attachments' && more.getAttribute('aria-pressed') === 'true';`),
      );
      await checkButtons(s);
      await s.click(await s.find('[role="toolbar"][aria-label="Filter messages"] > button'));
    });
  }
});
