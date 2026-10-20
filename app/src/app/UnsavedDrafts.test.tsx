// Drafts not yet saved head a Drafts list (docs/specs/ui.md, Message list):
// what maild holds is never out of sight before its server copy arrives.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { draftPreview } from "../features/list/UnsavedDraftRow";
import type { Draft, Mailbox } from "../rpc/gen/api";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";
import { draftsScope } from "./useUnsavedDrafts";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

afterEach(() => {
  vi.restoreAllMocks();
});

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  const list = await screen.findByRole("listbox", { name: "Messages" });
  // A reply as an agent drafts it: made, then given its text.
  const draft = async (subject: string, to = "ann@x.test") => {
    const d = await mock.call<Draft>("draft.create", { kind: "new" });
    await mock.call("draft.update", {
      id: d.id,
      content: { ...d.content, subject, to: [{ name: "Ann", address: to }], html: `<p>Yes to ${subject}</p>` },
    });
    return d.id;
  };
  const group = () => within(list).queryByRole("group", { name: "Not Yet Saved" });
  return { mock, list, draft, group };
}

describe("a Drafts list", () => {
  it("shows the drafts not yet saved, marked so, with no empty text, and opens one in Compose", async () => {
    const { list, draft, group } = await setup();
    const id = await draft("Saturday");
    useUI.getState().setSource({ kind: "mailbox", mailboxId: FIXTURE.drafts });
    const rows = await waitFor(() => {
      const g = group();
      if (!g) throw new Error("no unsaved drafts yet");
      return within(g).getAllByRole("option");
    });
    expect(rows).toHaveLength(1);
    const row = rows[0] as HTMLElement;
    expect(row.getAttribute("data-draft-id")).toBe(String(id));
    expect(row.textContent).toContain("To: Ann");
    expect(row.textContent).toContain("Saturday");
    expect(row.textContent).toContain("Not Saved");
    expect(row.textContent).toContain("Yes to Saturday");
    // The Drafts mailbox holds no message, and that is no empty mailbox.
    expect(within(list).queryByText(/No Messages/)).toBeNull();

    const open = vi.spyOn(window, "open").mockReturnValue(null);
    fireEvent.click(row);
    await waitFor(() => expect(open).toHaveBeenCalledOnce());
    expect(String(open.mock.calls[0]?.[0])).toMatch(new RegExp(`#/compose/${id}$`));
    fireEvent.keyDown(row, { key: "Enter" });
    await waitFor(() => expect(open).toHaveBeenCalledTimes(2));
  });

  it("shows every account's in All Drafts, newest first, and none in the inbox", async () => {
    const { draft, group } = await setup();
    await draft("First");
    await draft("Second");
    useUI.getState().setSource({ kind: "role", role: "drafts" });
    await waitFor(() => {
      const titles = within(group() ?? document.createElement("div"))
        .queryAllByRole("option")
        .map((r) => r.querySelector(".text-list-subject")?.textContent);
      expect(titles).toEqual(["Second", "First"]);
    });
    useUI.getState().setSource({ kind: "mailbox", mailboxId: FIXTURE.inbox });
    await waitFor(() => expect(group()).toBeNull());
  });

  it("follows drafts as they change: a new one appears, a sent or deleted one goes", async () => {
    const { mock, draft, group } = await setup();
    useUI.getState().setSource({ kind: "mailbox", mailboxId: FIXTURE.drafts });
    const sent = await draft("To send");
    const deleted = await draft("To delete");
    await waitFor(() => expect(within(group() ?? document.body).getAllByRole("option")).toHaveLength(2));
    await mock.call("draft.send", { id: sent });
    await mock.call("draft.delete", { id: deleted });
    await waitFor(() => expect(group()).toBeNull());
  });

  it("leaves out a draft opened from a stored message, which the list already holds", async () => {
    const { mock, draft, group } = await setup();
    const opened = await mock.call<Draft>("draft.open", { messageId: 1 });
    const kept = await draft("Kept");
    useUI.getState().setSource({ kind: "role", role: "drafts" });
    await waitFor(() => {
      const ids = within(group() ?? document.createElement("div"))
        .queryAllByRole("option")
        .map((r) => r.getAttribute("data-draft-id"));
      expect(ids).toEqual([String(kept)]);
    });
    expect(opened.id).not.toBe(kept);
  });
});

describe("draftsScope", () => {
  const mailboxes = [
    { id: 2, accountId: 1, role: "drafts" },
    { id: 9, accountId: 1, role: "inbox" },
  ] as Mailbox[];
  it("is the Drafts mailbox's account, every account for All Drafts, and none elsewhere", () => {
    expect(draftsScope({ kind: "mailbox", mailboxId: 2 }, mailboxes)).toEqual({ accountId: 1 });
    expect(draftsScope({ kind: "role", role: "drafts" }, mailboxes)).toEqual({});
    expect(draftsScope({ kind: "mailbox", mailboxId: 9 }, mailboxes)).toBeNull();
    expect(draftsScope({ kind: "role", role: "sent" }, mailboxes)).toBeNull();
    expect(draftsScope({ kind: "allInboxes" }, mailboxes)).toBeNull();
  });
});

describe("draftPreview", () => {
  it("is the draft's own text, without the quoted message", () => {
    expect(draftPreview("<p>Thanks,  Ann.</p><p>See you.</p><blockquote><p>Saturday?</p></blockquote>")).toBe(
      "Thanks, Ann. See you.",
    );
    expect(draftPreview("<p><br></p>")).toBe("");
  });
});
