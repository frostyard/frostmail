// Remote images (docs/design/rendering.md): Load Remote Content fetches them
// through maild, which caches them and embeds them in the rendering as
// data: URLs, since WebKitGTK loads no mailpart:// image inside the
// reader's frame. The image is put in maild's cache first, under the name
// maild gives its URL, so no network is needed.
import { createHash } from "node:crypto";
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { afterAll, beforeAll, describe, expect, it } from "vitest";

import { type App, fixture, launch, tempDir } from "./harness";

const IMAGE_URL = "https://images.example.test/photo.png";
// A 4 x 3 red PNG.
const PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAQAAAADCAIAAAA7ljmRAAAAEElEQVR4nGP4z8AARww4OQD1MQv1NXv7ggAAAABJRU5ErkJggg==",
  "base64",
);

let app: App | undefined;

beforeAll(async () => {
  const dir = tempDir("remote");
  const html = `<p>Photos from the trip</p><img alt="photo" src="${IMAGE_URL}" width="4" height="3">`;
  app = await launch(dir, fixture(dir, 3, { html: [{ name: "remote-photo.html", html }] }));
  const name = createHash("sha256").update(IMAGE_URL).digest("hex").slice(0, 32);
  mkdirSync(join(app.cache, "parts", "r"), { recursive: true, mode: 0o700 });
  writeFileSync(join(app.cache, "parts", "r", `${name}.png`), PNG);
  const s = app.session;
  await s.waitFor("the app", async () => (await s.findAll('[role="tree"]')).length > 0, 30_000);
});

afterAll(async () => {
  await app?.stop();
});

describe("remote images", () => {
  it("shows a remote image in the reader after Load Remote Content", async () => {
    if (!app) throw new Error("not started");
    const s = app.session;
    const mailbox = await s.waitFor("the Hostile mailbox", async () => {
      for (const el of await s.findAll('[role="treeitem"]')) if ((await s.text(el)).startsWith("Hostile")) return el;
      return undefined;
    });
    await s.click(mailbox);
    const row = await s.waitFor("the message", async () => (await s.findAll('[role="option"]'))[0]);
    await s.click(row);
    const load = await s.waitFor(
      "Load Remote Content",
      async () => (await s.findAll('section[aria-label="Message"] [role="status"] button'))[0],
    );
    await s.click(load);
    const width = await s.waitFor("the image to load in the frame", async () => {
      const w = await s.execute<number>(
        `const f = document.querySelector('section[aria-label="Message"] iframe');
         const img = f && f.contentDocument && f.contentDocument.querySelector("img");
         return img && img.complete && img.src.startsWith("data:image/png") ? img.naturalWidth : -1;`,
      );
      return w > 0 ? w : undefined;
    });
    expect(width).toBe(4);
  });
});
