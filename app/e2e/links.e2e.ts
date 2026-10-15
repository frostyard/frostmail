// Links in the reader (docs/adr/0036-reader-links-open-as-new-window-requests.md):
// a click on a link in the message frame opens it in the system browser
// through the app shell, and the frame and the app stay where they are.
// WebKitGTK runs none of the page's listeners inside the frame, so only the
// built app can show this. Stand-in launchers record what the app opens.
import { afterAll, beforeAll, describe, expect, it } from "vitest";

import { type App, fakeOpener, fixture, launch, type Opener, tempDir } from "./harness";

const WEB = "https://example.test/landing?from=frostmail";
const MAIL = "mailto:someone@example.test";

let app: App | undefined;
let opener: Opener | undefined;

beforeAll(async () => {
  const dir = tempDir("links");
  opener = fakeOpener(dir);
  const html = `<p>Read <a id="web" href="${WEB}">the <b>landing</b> page</a> or <a id="mail" href="${MAIL}">write to us</a>.</p>`;
  app = await launch(dir, fixture(dir, 3, { html: [{ name: "links.html", html }] }), opener.env);
  const s = app.session;
  await s.waitFor("the app", async () => (await s.findAll('[role="tree"]')).length > 0, 30_000);
});

afterAll(async () => {
  await app?.stop();
});

describe("links in the reader", () => {
  it("open in the system browser, leaving the frame and the app as they were", async () => {
    if (!app || !opener) throw new Error("not started");
    const s = app.session;
    const opened = opener.opened;
    const mailbox = await s.waitFor("the Hostile mailbox", async () => {
      for (const el of await s.findAll('[role="treeitem"]')) if ((await s.text(el)).startsWith("Hostile")) return el;
      return undefined;
    });
    await s.click(mailbox);
    const row = await s.waitFor("the message", async () => (await s.findAll('[role="option"]'))[0]);
    await s.click(row);
    await s.waitFor("the links in the sized frame", () =>
      s.execute<boolean>(
        `const f = document.querySelector('section[aria-label="Message"] iframe');
         return !!(f && f.contentDocument && f.contentDocument.querySelector("#mail") && f.offsetHeight > 0);`,
      ),
    );
    await s.frame(await s.find('section[aria-label="Message"] iframe'));
    // A click anywhere inside the link, as on its bold word, opens it.
    await s.click(await s.find("#web b"));
    await s.waitFor("the web link to open", async () => opened().includes(WEB));
    await s.click(await s.find("#mail"));
    await s.waitFor("the mail link to open", async () => opened().includes(MAIL));
    const where = await s.execute<string>("return location.href");
    await s.parentFrame();
    expect(where).toBe("about:srcdoc");
    expect(await s.windows()).toHaveLength(1);
    expect(opened()).toEqual([WEB, MAIL]);
  });
});
