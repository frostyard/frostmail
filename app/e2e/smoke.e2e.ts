// The app starts on a fixture maild, lists the inbox and reads a message.
import { writeFileSync } from "node:fs";
import { join } from "node:path";

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import { type App, BUILD, fixture, launch, tempDir } from "./harness";

let app: App | undefined;

beforeAll(async () => {
  const dir = tempDir("smoke");
  app = await launch(dir, fixture(dir, 200));
});

afterAll(async () => {
  await app?.stop();
});

describe("the app", () => {
  it("lists the inbox and reads a message", async () => {
    if (!app) throw new Error("app did not start");
    const s = app.session;
    await s.waitFor("message rows", async () => (await s.findAll('[role="option"]')).length > 5, 30_000);
    const toolbar = await s.text(await s.find('[role="toolbar"]'));
    expect(toolbar).toContain("All Inboxes");
    const [first] = await s.findAll('[role="option"]');
    if (!first) throw new Error("no rows");
    await s.click(first);
    const header = await s.waitFor("the reader header", async () => {
      const found = await s.findAll('section[aria-label="Message"] header');
      return found[0];
    });
    expect((await s.text(header)).length).toBeGreaterThan(0);
    writeFileSync(join(BUILD, "e2e-smoke.png"), Buffer.from(await s.screenshot(), "base64"));
  });
});
