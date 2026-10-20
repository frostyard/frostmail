// Drafts not yet saved head a Drafts list (docs/specs/ui.md, Message list):
// what maild holds is never out of sight before its server copy arrives.
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { draftPreview } from "../features/list/UnsavedDraftRow";
import { type Draft, ErrorCode, type Mailbox } from "../rpc/gen/api";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { RPCError } from "../rpc/transport";
import { MainWindow } from "./MainWindow";
import { draftsScope } from "./useUnsavedDrafts";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

afterEach(() => {
  vi.restoreAllMocks();
});

/** OTHER is a second account's Drafts mailbox, when setup adds the account. */
const OTHER = { accountId: 2, drafts: 20 };

async function setup(twoAccounts = false) {
  const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") });
  if (twoAccounts) {
    const first = data.accounts[0];
    if (first) data.accounts.push({ ...first, id: OTHER.accountId, email: "two@mailtest.test", displayName: "Two" });
    data.mailboxes.push({
      id: OTHER.drafts,
      accountId: OTHER.accountId,
      path: "Drafts",
      name: "Drafts",
      delimiter: "/",
      role: "drafts",
      label: false,
    });
  }
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  const list = await screen.findByRole("listbox", { name: "Messages" });
  // A reply as an agent drafts it: made, then given its text.
  const draft = async (subject: string, to = "ann@x.test", accountId: number = FIXTURE.accountId) => {
    const d = await mock.call<Draft>("draft.create", { kind: "new", accountId });
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
    // The heading is on screen, not only the group's name.
    expect(within(group() as HTMLElement).getByText("Not Yet Saved")).toBeTruthy();
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

  it("never shows another account's drafts in a Drafts mailbox, even while its own load", async () => {
    const { draft, group } = await setup(true);
    const mine = await draft("Mine");
    const theirs = await draft("Theirs", "bob@x.test", OTHER.accountId);
    useUI.getState().setSource({ kind: "role", role: "drafts" });
    const ids = () =>
      within(group() ?? document.createElement("div"))
        .queryAllByRole("option")
        .map((r) => Number(r.getAttribute("data-draft-id")));
    await waitFor(() => expect(ids().sort()).toEqual([mine, theirs].sort()));
    // The switch renders before maild answers for the account: no draft of
    // the other account may show meanwhile.
    act(() => useUI.getState().setSource({ kind: "mailbox", mailboxId: FIXTURE.drafts }));
    expect(ids()).not.toContain(theirs);
    await waitFor(() => expect(ids()).toEqual([mine]));
    act(() => useUI.getState().setSource({ kind: "mailbox", mailboxId: OTHER.drafts }));
    expect(ids()).not.toContain(mine);
    await waitFor(() => expect(ids()).toEqual([theirs]));
  });

  it("says so when the drafts not yet saved cannot be listed, rather than showing an empty Drafts", async () => {
    const { mock, list } = await setup();
    const call = mock.call.bind(mock);
    vi.spyOn(mock, "call").mockImplementation((method, params) =>
      method === "draft.unsaved"
        ? Promise.reject(new RPCError(ErrorCode.methodNotFound, "method not found"))
        : call(method, params),
    );
    vi.spyOn(console, "warn").mockImplementation(() => {});
    useUI.getState().setSource({ kind: "mailbox", mailboxId: FIXTURE.drafts });
    const alert = await within(list).findByRole("alert");
    expect(alert.textContent).toContain("could not be listed");
    expect(within(list).getByText("Not Yet Saved")).toBeTruthy();
    expect(within(list).queryByText(/No Messages/)).toBeNull();
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
