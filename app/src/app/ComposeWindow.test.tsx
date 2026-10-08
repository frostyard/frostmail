import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { Client, type DraftContent } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { ComposeWindow, SAVE_MS } from "./ComposeWindow";

async function setup(opts: { fresh?: boolean; content?: Partial<DraftContent> } = {}) {
  const mock = new MockTransport(mockData({ inbox: 3, now: new Date("2026-10-07T12:00:00Z") }), { undoMs: 50 });
  const client = new Client(mock);
  const draft = await client.draft.create({ kind: "new" });
  if (opts.content) await client.draft.update({ id: draft.id, content: { ...draft.content, ...opts.content } });
  const close = vi.spyOn(window, "close").mockImplementation(() => {});
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <ComposeWindow draftId={draft.id} fresh={opts.fresh ?? false} />
    </Session>,
  );
  await screen.findByRole("toolbar", { name: "Compose" });
  const calls = (method: string) => mock.calls.filter((c) => c.method === method);
  return { mock, client, draft, close, calls };
}

const button = (name: string) => screen.getByRole("button", { name }) as HTMLButtonElement;

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  document.title = "";
});

describe("ComposeWindow", () => {
  it("shows the draft and titles the window with its subject", async () => {
    await setup({ content: { subject: "Lunch", to: [{ name: "Bob", address: "bob@x.test" }] } });
    expect((screen.getByRole("textbox", { name: "Subject" }) as HTMLInputElement).value).toBe("Lunch");
    expect(screen.getByRole("button", { name: "Bob" }).getAttribute("title")).toBe("Bob <bob@x.test>");
    await waitFor(() => expect(document.title).toBe("Lunch"));
    expect(screen.getByRole("textbox", { name: "Message body" })).toBeTruthy();
  });

  it("saves edits after a pause", async () => {
    const user = userEvent.setup();
    const { calls } = await setup();
    await user.type(screen.getByRole("textbox", { name: "Subject" }), "Hi");
    expect(calls("draft.update")).toHaveLength(0);
    await waitFor(() => expect(calls("draft.update")).toHaveLength(1), { timeout: SAVE_MS * 4 });
    const params = calls("draft.update")[0]?.params as { content: DraftContent };
    expect(params.content.subject).toBe("Hi");
  });

  it("sends only with valid recipients, then closes", async () => {
    const user = userEvent.setup();
    const { calls, close } = await setup({ content: { subject: "Go" } });
    expect(button("Send").disabled).toBe(true);
    await user.type(screen.getByRole("combobox", { name: "To" }), "nope,");
    expect(button("Send").disabled).toBe(true);
    await user.click(screen.getByRole("button", { name: "nope" }));
    await user.keyboard("{Delete}");
    await user.type(screen.getByRole("combobox", { name: "To" }), "bob@x.test,");
    expect(button("Send").disabled).toBe(false);
    await user.click(button("Send"));
    await waitFor(() => expect(close).toHaveBeenCalled());
    const update = calls("draft.update").at(-1)?.params as { content: DraftContent };
    expect(update.content.to).toEqual([{ name: "", address: "bob@x.test" }]);
    expect(calls("draft.send")).toHaveLength(1);
  });

  it("shows a refused send and stays open", async () => {
    const user = userEvent.setup();
    const { mock, close } = await setup({ content: { to: [{ name: "", address: "bob@x.test" }] } });
    const original = mock.call.bind(mock);
    vi.spyOn(mock, "call").mockImplementation((method: string, params: unknown) =>
      method === "draft.send" ? Promise.reject(new Error("the server is unhappy")) : original(method, params),
    );
    await user.click(button("Send"));
    expect((await screen.findByRole("alert")).textContent).toContain("the server is unhappy");
    expect(close).not.toHaveBeenCalled();
    expect(button("Send").disabled).toBe(false);
  });

  it("discards a fresh draft closed untouched", async () => {
    const user = userEvent.setup();
    const { calls, close } = await setup({ fresh: true });
    await user.click(button("Close"));
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(calls("draft.delete")).toHaveLength(1);
  });

  it("keeps a reopened draft closed untouched, saving pending edits first", async () => {
    const user = userEvent.setup();
    const { calls, close } = await setup({ fresh: false });
    await user.click(button("Close"));
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(calls("draft.delete")).toHaveLength(0);

    close.mockClear();
    await user.type(screen.getByRole("textbox", { name: "Subject" }), "x");
    await user.click(button("Close"));
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(calls("draft.update").length).toBeGreaterThan(0);
  });

  it("deletes an empty draft without asking", async () => {
    const user = userEvent.setup();
    const confirm = vi.fn(() => true);
    vi.stubGlobal("confirm", confirm);
    const { calls, close } = await setup();
    await user.click(button("Delete Draft"));
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(confirm).not.toHaveBeenCalled();
    expect(calls("draft.delete")).toHaveLength(1);
  });

  it("asks before deleting a draft with content", async () => {
    const user = userEvent.setup();
    const confirm = vi.fn(() => false);
    vi.stubGlobal("confirm", confirm);
    const { calls, close } = await setup({ content: { subject: "Keep me" } });
    await user.click(button("Delete Draft"));
    expect(confirm).toHaveBeenCalled();
    expect(calls("draft.delete")).toHaveLength(0);
    expect(close).not.toHaveBeenCalled();
  });

  it("shows Bcc on Ctrl+Shift+B and the format bar from the toolbar", async () => {
    const user = userEvent.setup();
    await setup();
    expect(screen.queryByRole("combobox", { name: "Bcc" })).toBeNull();
    await user.keyboard("{Control>}{Shift>}b{/Shift}{/Control}");
    expect(screen.getByRole("combobox", { name: "Bcc" })).toBeTruthy();
    await user.click(button("Show Format Bar"));
    expect(screen.getByRole("toolbar", { name: "Format" })).toBeTruthy();
    expect(button("Show Format Bar").getAttribute("aria-pressed")).toBe("true");
  });
});
