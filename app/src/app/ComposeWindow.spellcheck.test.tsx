// Spelling is checked in the words the user writes, never in addresses
// (docs/specs/compose-ui.md, Spelling).
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { ComposeWindow } from "./ComposeWindow";

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 3, now: new Date("2026-10-07T12:00:00Z") }), { undoMs: 50 });
  const client = new Client(mock);
  const draft = await client.draft.create({ kind: "new" });
  // A Bcc address shows the Bcc field from the start.
  await client.draft.update({
    id: draft.id,
    content: { ...draft.content, subject: "Lunch", bcc: [{ name: "Carol", address: "carol@x.test" }] },
  });
  vi.spyOn(window, "close").mockImplementation(() => {});
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <ComposeWindow draftId={draft.id} fresh={false} />
    </Session>,
  );
  await screen.findByRole("toolbar", { name: "Compose" });
}

afterEach(() => {
  vi.restoreAllMocks();
  document.title = "";
});

describe("spelling in the compose window", () => {
  it("is checked in the subject and the body", async () => {
    await setup();
    expect(screen.getByRole("textbox", { name: "Subject" }).getAttribute("spellcheck")).toBe("true");
    expect(screen.getByRole("textbox", { name: "Message body" }).getAttribute("spellcheck")).toBe("true");
  });

  it("is not checked in To, Cc or Bcc", async () => {
    await setup();
    for (const name of ["To", "Cc", "Bcc"]) {
      expect(screen.getByRole("combobox", { name }).getAttribute("spellcheck"), name).toBe("false");
    }
  });

  it("is not checked in a link's URL", async () => {
    const user = userEvent.setup();
    await setup();
    await user.keyboard("{Control>}k{/Control}");
    expect((await screen.findByRole("textbox", { name: "Link URL" })).getAttribute("spellcheck")).toBe("false");
  });
});
