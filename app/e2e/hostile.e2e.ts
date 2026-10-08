// The hostile-HTML suite (docs/plans/0004-m2-ui-read-path.md, exit criterion
// 3): every corpus message, pointed at a canary server, must make zero
// requests, both raw in the reader's frame (sandbox and CSP alone) and
// through maild's sanitizer in the real reader, with remote content allowed.
import { afterAll, beforeAll, describe, expect, it } from "vitest";

import { type App, type Canary, canary, fixture, hostileFiles, launch, tempDir } from "./harness";

let app: App | undefined;
let trap: Canary | undefined;

beforeAll(async () => {
  trap = await canary();
  const dir = tempDir("hostile");
  app = await launch(dir, fixture(dir, 5, { canaryOrigin: trap.origin }));
  const s = app.session;
  await s.waitFor("the app", async () => (await s.findAll('[role="tree"]')).length > 0, 30_000);
});

afterAll(async () => {
  await app?.stop();
  await trap?.close();
});

const settle = (ms: number) => new Promise((r) => setTimeout(r, ms));

describe("hostile HTML", () => {
  it("makes no requests raw in a sandboxed frame under the app CSP", async () => {
    if (!app || !trap) throw new Error("not started");
    const files = hostileFiles(trap.origin);
    expect(files.length).toBeGreaterThanOrEqual(40);
    for (const f of files) {
      // The same frame MessageFrame makes, without maild's sanitizer.
      await app.session.executeAsync(
        `const [html, done] = arguments;
         const frame = document.createElement("iframe");
         frame.setAttribute("sandbox", "allow-same-origin");
         frame.srcdoc = html;
         document.body.appendChild(frame);
         setTimeout(() => { frame.remove(); done(); }, 400);`,
        f.html,
      );
    }
    await settle(2000);
    expect(trap.hits).toEqual([]);
  });

  it("makes no requests through maild's sanitizer, even with remote content loaded", async () => {
    if (!app || !trap) throw new Error("not started");
    const s = app.session;
    const mailbox = await s.waitFor("the Hostile mailbox", async () => {
      for (const el of await s.findAll('[role="treeitem"]')) if ((await s.text(el)).startsWith("Hostile")) return el;
      return undefined;
    });
    await s.click(mailbox);
    const count = hostileFiles(trap.origin).length;
    await s.waitFor(
      "every hostile message",
      async () => (await s.findAll('[role="option"]')).length >= Math.min(count, 8),
    );
    const shown = new Set<string>();
    for (let i = 0; i < count; i++) {
      // Select row i by keyboard from the first, so rows scrolled out of view
      // are reached.
      if (i === 0) {
        const [first] = await s.findAll('[role="option"]');
        if (first) await s.click(first);
      } else {
        // Loading remote content moved focus to the reader; give it back to
        // the list (a separate call, so the app sees the focus change first).
        await s.execute(`document.querySelector('[role="listbox"]').focus()`);
        await settle(50);
        await s.execute(
          `document.querySelector('[role="listbox"]').dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }))`,
        );
      }
      await settle(300);
      const subject = await s.execute<string | null>(
        `return document.querySelector('section[aria-label="Message"] article')?.getAttribute("aria-label") ?? null`,
      );
      if (subject) shown.add(subject);
      const load = await s.findAll('section[aria-label="Message"] [role="status"] button');
      for (const b of load) await s.click(b).catch(() => {});
      await settle(300);
      const frames = await s.execute<string[]>(
        `return [...document.querySelectorAll('section[aria-label="Message"] iframe')].map((f) => f.getAttribute("sandbox"))`,
      );
      expect(frames.every((sb) => sb === "allow-same-origin")).toBe(true);
    }
    await settle(2000);
    // Every corpus message was opened, not the first one again and again.
    expect(shown.size).toBe(count);
    expect(trap.hits).toEqual([]);
  });
});
