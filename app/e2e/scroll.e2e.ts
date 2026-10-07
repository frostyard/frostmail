// Scrolling a 100,000-row Inbox (docs/plans/0004-m2-ui-read-path.md, exit
// criterion 2): frame times while scrolling and no blank rows once it stops.
// The budget is for the nsl machine's software rendering under Xvfb.
import { afterAll, beforeAll, describe, expect, it } from "vitest";

import { type App, fixture, launch, tempDir } from "./harness";

const ROWS = 100_000;
const P95_BUDGET_MS = 25;

let app: App | undefined;

beforeAll(async () => {
  const dir = tempDir("scroll");
  app = await launch(dir, fixture(dir, ROWS));
  const s = app.session;
  await s.waitFor("message rows", async () => (await s.findAll('[role="option"]')).length > 5, 60_000);
});

afterAll(async () => {
  await app?.stop();
});

interface Run {
  frames: number;
  p95: number;
  max: number;
  blank: number;
  scrolled: number;
}

describe("the message list", () => {
  it(`scrolls ${ROWS.toLocaleString("en-US")} rows smoothly`, async () => {
    if (!app) throw new Error("app did not start");
    // Scroll 300 frames at 400px a frame (about 5 rows), then let rows load
    // and count visible slots without a row.
    const run = await app.session.executeAsync<Run>(
      `const done = arguments[arguments.length - 1];
       const list = document.querySelector('[role="listbox"]');
       const times = [];
       let last = performance.now();
       let n = 0;
       const step = (now) => {
         times.push(now - last);
         last = now;
         list.scrollTop += 400;
         if (++n < 300) { requestAnimationFrame(step); return; }
         setTimeout(() => {
           const box = list.getBoundingClientRect();
           let blank = 0;
           for (let y = box.top + 42; y < box.bottom; y += 84) {
             const el = document.elementFromPoint(box.left + box.width / 2, y);
             if (!el || !el.closest('[role="option"]')) blank++;
           }
           const sorted = times.slice(1).sort((a, b) => a - b);
           done({ frames: sorted.length, p95: sorted[Math.floor(sorted.length * 0.95)], max: sorted[sorted.length - 1], blank, scrolled: list.scrollTop });
         }, 1500);
       };
       requestAnimationFrame((now) => { last = now; requestAnimationFrame(step); });`,
    );
    console.log(
      `scroll: ${run.frames} frames, p95 ${run.p95.toFixed(1)} ms, max ${run.max.toFixed(1)} ms, ${run.scrolled}px, ${run.blank} blank slots`,
    );
    expect(run.scrolled).toBeGreaterThan(100_000);
    expect(run.blank).toBe(0);
    expect(run.p95).toBeLessThan(P95_BUDGET_MS);
  });
});
