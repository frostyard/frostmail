import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { atLocal, sendChoices } from "../lib/later";
import { openFrameWindow } from "../lodge/frameWindow";
import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { closeWatch } from "../testing/frame";
import { ComposeWindow } from "./ComposeWindow";

async function setup(
  html: string,
  opts: { signature?: string; attach?: boolean; holdIdentities?: Promise<void>; failIdentities?: boolean } = {},
) {
  const mock = new MockTransport(mockData({ inbox: 3, now: new Date("2026-10-07T12:00:00Z") }), { undoMs: 50 });
  const client = new Client(mock);
  const draft = await client.draft.create({ kind: "new" });
  await client.draft.update({
    id: draft.id,
    content: { ...draft.content, subject: "Hi", html, to: [{ name: "Bob", address: "bob@x.test" }] },
  });
  if (opts.signature !== undefined) {
    await client.identity.update({ id: draft.content.identityId, signatureHtml: opts.signature });
  }
  if (opts.attach) await client.draft.attach({ id: draft.id, path: "/tmp/a.txt" });
  if (opts.holdIdentities || opts.failIdentities) {
    const hold = opts.holdIdentities;
    const original = mock.call.bind(mock);
    vi.spyOn(mock, "call").mockImplementation(async (method: string, params: unknown) => {
      if (method === "identity.list") {
        if (hold) await hold;
        if (opts.failIdentities) throw new Error("identity.list failed");
      }
      return original(method, params);
    });
  }
  openFrameWindow({ kind: "compose", draftId: draft.id, fresh: false });
  const close = closeWatch();
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <ComposeWindow draftId={draft.id} fresh={false} />
    </Session>,
  );
  await screen.findByRole("toolbar", { name: "Compose" });
  if (!opts.holdIdentities) {
    await waitFor(() =>
      expect((screen.getByRole("button", { name: "Send" }) as HTMLButtonElement).disabled).toBe(false),
    );
  }
  if (opts.attach) await screen.findByText("a.txt");
  const sends = () => mock.calls.filter((c) => c.method === "draft.send").map((c) => c.params);
  return { mock, draft, close, sends };
}

const SEE = "<p>see attached</p>";
const tomorrow = () =>
  (sendChoices(new Date(), Intl.DateTimeFormat().resolvedOptions().timeZone, "en-US").at(-1) as { at: Date }).at;
const TITLE = "Send without an attachment?";

afterEach(() => {
  vi.restoreAllMocks();
  document.title = "";
});

describe("attachment reminder in the compose window", () => {
  it("opens the dialog and does not send", async () => {
    const user = userEvent.setup();
    const { sends } = await setup(SEE);
    await user.click(screen.getByRole("button", { name: "Send" }));
    const dialog = await screen.findByRole("dialog", { name: TITLE });
    expect(within(dialog).getByText("Your message mentions an attachment, but nothing is attached.")).toBeTruthy();
    await waitFor(() =>
      expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: "Attach Files…" })),
    );
    expect(sends()).toEqual([]);
  });

  it("Ctrl+Enter asks too, and Escape cancels", async () => {
    const user = userEvent.setup();
    const { sends, close } = await setup(SEE);
    await user.keyboard("{Control>}{Enter}{/Control}");
    await screen.findByRole("dialog", { name: TITLE });
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog", { name: TITLE })).toBeNull());
    expect(sends()).toEqual([]);
    expect(close).not.toHaveBeenCalled();
  });

  it("Send Anyway sends once", async () => {
    const user = userEvent.setup();
    const { sends, close } = await setup(SEE);
    await user.click(screen.getByRole("button", { name: "Send" }));
    await user.click(
      within(await screen.findByRole("dialog", { name: TITLE })).getByRole("button", { name: "Send Anyway" }),
    );
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(sends()).toHaveLength(1);
  });

  it("Send Anyway keeps the Send Later choice", async () => {
    const user = userEvent.setup();
    const { sends, draft } = await setup(SEE);
    await user.click(screen.getByRole("button", { name: "Send Later" }));
    await user.click(within(screen.getByRole("menu")).getByText(/^Send .+ Tomorrow$/));
    expect(sends()).toEqual([]);
    await user.click(
      within(await screen.findByRole("dialog", { name: TITLE })).getByRole("button", { name: "Send Anyway" }),
    );
    await waitFor(() => expect(sends()).toHaveLength(1));
    expect(sends()).toEqual([{ id: draft.id, sendAt: tomorrow().toISOString() }]);
  });

  it("Attach Files… opens the file picker and sends nothing", async () => {
    const user = userEvent.setup();
    const { sends } = await setup(SEE);
    const click = vi.spyOn(HTMLInputElement.prototype, "click").mockImplementation(() => {});
    await user.click(screen.getByRole("button", { name: "Send" }));
    await user.click(
      within(await screen.findByRole("dialog", { name: TITLE })).getByRole("button", { name: "Attach Files…" }),
    );
    await waitFor(() => expect(click).toHaveBeenCalled());
    expect(screen.queryByRole("dialog", { name: TITLE })).toBeNull();
    expect(sends()).toEqual([]);
  });

  it("Cancel sends nothing", async () => {
    const user = userEvent.setup();
    const { sends, close } = await setup(SEE);
    await user.click(screen.getByRole("button", { name: "Send" }));
    await user.click(
      within(await screen.findByRole("dialog", { name: TITLE })).getByRole("button", { name: "Cancel" }),
    );
    expect(screen.queryByRole("dialog", { name: TITLE })).toBeNull();
    expect(sends()).toEqual([]);
    expect(close).not.toHaveBeenCalled();
  });

  it("sends with an attachment chip", async () => {
    const user = userEvent.setup();
    const { sends } = await setup(SEE, { attach: true });
    await user.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(sends()).toHaveLength(1));
    expect(screen.queryByRole("dialog", { name: TITLE })).toBeNull();
  });

  it("sends with an image outside a quote", async () => {
    const user = userEvent.setup();
    const { sends } = await setup('<p>see attached</p><p><img src="data:image/png;base64,AA"></p>');
    await user.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(sends()).toHaveLength(1));
    expect(screen.queryByRole("dialog", { name: TITLE })).toBeNull();
  });

  it("sends when attached is only in a quote", async () => {
    const user = userEvent.setup();
    const { sends } = await setup(
      '<p>Ok</p><p>On x, Ann wrote:</p><blockquote type="cite"><p>see attached</p></blockquote>',
    );
    await user.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(sends()).toHaveLength(1));
    expect(screen.queryByRole("dialog", { name: TITLE })).toBeNull();
  });

  it("sends when attached is only in the signature", async () => {
    const user = userEvent.setup();
    const sig = "<p>Ann</p><p>Attached Corp</p>";
    const { sends } = await setup(`<p>Ok</p><p><br></p>${sig}`, { signature: sig });
    await user.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(sends()).toHaveLength(1));
    expect(screen.queryByRole("dialog", { name: TITLE })).toBeNull();
  });

  it("waits for the identity list before it decides", async () => {
    const user = userEvent.setup();
    let release: () => void = () => {};
    const hold = new Promise<void>((resolve) => {
      release = resolve;
    });
    const sig = "<p>Ann</p><p>Attached Corp</p>";
    const { sends } = await setup(`<p>Ok</p><p><br></p>${sig}`, { signature: sig, holdIdentities: hold });
    const send = screen.getByRole("button", { name: "Send" }) as HTMLButtonElement;
    expect(send.disabled).toBe(true);
    await user.keyboard("{Control>}{Enter}{/Control}");
    expect(sends()).toEqual([]);
    release();
    await waitFor(() => expect(send.disabled).toBe(false));
    await user.click(send);
    await waitFor(() => expect(sends()).toHaveLength(1));
    expect(screen.queryByRole("dialog", { name: TITLE })).toBeNull();
  });

  it("a failed identity list still asks", async () => {
    const user = userEvent.setup();
    const { sends } = await setup("<p>Ok</p><p><br></p><p>Ann</p><p>Attached Corp</p>", { failIdentities: true });
    await user.click(screen.getByRole("button", { name: "Send" }));
    await screen.findByRole("dialog", { name: TITLE });
    expect(sends()).toEqual([]);
  });

  it("sends once on a double click", async () => {
    const user = userEvent.setup();
    const { sends, close } = await setup(SEE, { attach: true });
    const send = screen.getByRole("button", { name: "Send" });
    await user.dblClick(send);
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(sends()).toHaveLength(1);
  });

  it("checks the Send Later time sheet's OK", async () => {
    const user = userEvent.setup();
    const { sends, draft } = await setup(SEE);
    await user.click(screen.getByRole("button", { name: "Send Later" }));
    await user.click(within(screen.getByRole("menu")).getByText("Send Later…"));
    const sheet = screen.getByRole("dialog", { name: "Send Later" });
    const date = within(sheet).getByLabelText("Date");
    await user.clear(date);
    await user.type(date, "2030-01-02");
    const time = within(sheet).getByLabelText("Time");
    await user.clear(time);
    await user.type(time, "09:30");
    await user.click(within(sheet).getByRole("button", { name: "OK" }));
    const dialog = await screen.findByRole("dialog", { name: TITLE });
    expect(sends()).toEqual([]);
    await user.click(within(dialog).getByRole("button", { name: "Send Anyway" }));
    await waitFor(() => expect(sends()).toHaveLength(1));
    const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    expect(sends()).toEqual([{ id: draft.id, sendAt: atLocal("2030-01-02", 9, 30, zone).toISOString() }]);
  });
});
