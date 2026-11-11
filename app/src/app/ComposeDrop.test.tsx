// A compose window attaches a file by its bytes: maild cannot read a path on
// this machine (docs/specs/compose-ui.md, Attachments).
import { createEvent, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { ComposeWindow } from "./ComposeWindow";

/** setup renders a new draft's compose window on the mock transport. */
async function setup(): Promise<MockTransport> {
  const mock = new MockTransport(mockData({ inbox: 1 }));
  const draft = await new Client(mock).draft.create({ kind: "new" });
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <ComposeWindow draftId={draft.id} fresh={false} />
    </Session>,
  );
  await screen.findByRole("toolbar", { name: "Compose" });
  return mock;
}

describe("ComposeWindow's attachments", () => {
  it("attaches a dropped file by its bytes", async () => {
    const mock = await setup();
    const file = new File(["hello"], "notes.txt", { type: "text/plain" });
    const drop = createEvent.drop(window, { dataTransfer: { types: ["Files"], files: [file] } });
    fireEvent(window, drop);
    await waitFor(() => expect(mock.calls.some((c) => c.method === "draft.attachData")).toBe(true));
    expect(mock.calls.some((c) => c.method === "draft.attach")).toBe(false);
  });

  it("uses the page's own chooser for Attach Files, and sends its file by bytes", async () => {
    const mock = await setup();
    const input = screen.getByLabelText("Files to attach") as HTMLInputElement;
    const click = vi.spyOn(input, "click").mockImplementation(() => {});
    fireEvent.click(screen.getByRole("button", { name: "Attach Files" }));
    await waitFor(() => expect(click).toHaveBeenCalled());

    fireEvent.change(input, { target: { files: [new File(["hello"], "notes.txt", { type: "text/plain" })] } });
    await waitFor(() => expect(mock.calls.some((c) => c.method === "draft.attachData")).toBe(true));
    expect(mock.calls.some((c) => c.method === "draft.attach")).toBe(false);
  });
});
