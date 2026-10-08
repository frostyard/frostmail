// Compose, send and undo through the built app: a compose window opens from
// the main window, the message goes to tools/smtpsink after the undo delay,
// and Undo in the main window brings the draft back. Attaching through the
// native file dialog cannot be driven over WebDriver; maild's attachments
// are covered by its integration tests (internal/mailsync).
import { writeFileSync } from "node:fs";
import { join } from "node:path";

import { afterAll, beforeAll, describe, expect, it } from "vitest";

import { type App, BUILD, fixture, launch, type Sink, smtpSink, tempDir } from "./harness";
import type { Session } from "./webdriver";

let app: App | undefined;
let sink: Sink | undefined;

beforeAll(async () => {
  const dir = tempDir("compose");
  sink = await smtpSink(dir);
  app = await launch(dir, fixture(dir, 20, { smtp: sink.addr }), { FROSTMAIL_UNDO_DELAY: "4s" });
}, 60_000);

afterAll(async () => {
  await app?.stop();
  sink?.stop();
});

/** otherWindow waits for a window besides main and switches to it. */
async function otherWindow(s: Session, main: string): Promise<string> {
  const handle = await s.waitFor("a compose window", async () => (await s.windows()).find((h) => h !== main));
  await s.switchTo(handle);
  await s.waitFor("the compose toolbar", () => s.find('[role="toolbar"][aria-label="Compose"]'));
  return handle;
}

/** backToMain switches to the main window once the compose window has closed. */
async function backToMain(s: Session, main: string): Promise<void> {
  await s.waitFor("the compose window to close", async () => (await s.windows()).length === 1);
  await s.switchTo(main);
}

describe("composing", () => {
  it("sends after the undo delay, and Undo brings the draft back", async () => {
    if (!app || !sink) throw new Error("app did not start");
    const s = app.session;
    const main = await s.window();
    await s.waitFor("message rows", async () => (await s.findAll('[role="option"]')).length > 5, 30_000);

    await s.click(await s.find('button[aria-label="New Message"]'));
    await otherWindow(s, main);
    await s.type(await s.find('input[aria-label="To"]'), "Bob <bob@x.test>,");
    await s.type(await s.find('input[aria-label="Subject"]'), "E2E hello");
    const body = await s.find(".compose-body");
    await s.click(body);
    await s.type(body, "Typed in the compose window.");
    await s.waitFor("Send to be enabled", async () =>
      s.execute<boolean>('return !document.querySelector("button[aria-label=Send]").disabled'),
    );
    await s.click(await s.find('button[aria-label="Send"]'));
    await backToMain(s, main);

    const toast = await s.waitFor("the undo toast", () => s.find("output"));
    expect(await s.text(toast)).toContain("Sending “E2E hello”");
    await s.click(await s.find("output button"));
    await otherWindow(s, main);
    const subject = await s.execute<string>('return document.querySelector("input[aria-label=Subject]").value');
    expect(subject).toBe("E2E hello");
    writeFileSync(join(BUILD, "e2e-compose.png"), Buffer.from(await s.screenshot(), "base64"));
    expect(sink.messages()).toHaveLength(0);

    await s.click(await s.find('button[aria-label="Send"]'));
    await backToMain(s, main);
    const sent = await s.waitFor("the message at the SMTP sink", async () => sink?.messages()[0], 20_000);
    expect(sent).toContain("Subject: E2E hello");
    expect(sent).toContain('To: "Bob" <bob@x.test>');
    expect(sent).toContain("Typed in the compose window.");
    expect(sink.messages()).toHaveLength(1);
  });
});
