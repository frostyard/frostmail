import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useMail, useUI } from "../data/stores";
import { type Address, Client, type MessageSummary } from "../rpc/gen/api";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { type MockMessage, MockTransport } from "../rpc/mock/mock";
import { toggleFlag, toggleRead } from "./commands";
import { MainWindow } from "./MainWindow";

const ME: Address = { name: "Test One", address: "test1@mailtest.test" };
const ANN: Address = { name: "Ann Smith", address: "ann.smith@northwind.test" };
const BOB: Address = { name: "Bob Okafor", address: "bob.okafor@acme.test" };

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  useMail.setState({ outbox: [] });
});

/** sent is a message in Sent, from the account to someone else, in a thread of its own. */
function sent(id: number, threadId: number, subject: string, to: Address, daysAgo = 4): MockMessage {
  const summary: MessageSummary = {
    id,
    accountId: FIXTURE.accountId,
    mailboxIds: [FIXTURE.sent],
    threadId,
    subject,
    from: ME,
    date: new Date(Date.now() - daysAgo * 86_400_000).toISOString(),
    preview: "",
    flags: { seen: true, flagged: false, answered: false, forwarded: false, draft: false, flagColor: 0 },
    hasAttachments: false,
    size: 2000,
    threadCount: 1,
  };
  return { summary, to: [to], cc: [], parts: [], text: "", html: "", remote: 0, trackers: 0 };
}

function setup(messages: MockMessage[]) {
  const data = mockData({ inbox: 3 });
  // The fixture's own mail is all from someone else, or to the account itself.
  data.messages.push(...messages);
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  return { mock, calls: () => mock.calls.filter((c) => c.method === "thread.awaitingOthers").map((c) => c.params) };
}

/**
 * pinned reveals a message 4 days old that is past the first page of Sent:
 * 150 newer sent messages fill it.
 */
async function pinned() {
  const newer = Array.from({ length: 150 }, (_, i) => sent(7000 + i, 7000 + i, `Newer ${i}`, ANN, (i + 1) / 1440));
  const { mock } = setup([...newer, sent(9001, 9001, "Offsite plan", ANN, 4)]);
  const section = await screen.findByRole("region", { name: "Follow Up" });
  const before = mock.calls.length;
  fireEvent.click(await within(section).findByText("Offsite plan"));
  // The Sent view opens and answers its first page, which holds no row for it.
  await waitFor(() =>
    expect(
      mock.calls.slice(before).some((c) => c.method === "view.range" && (c.params as { start?: number }).start === 0),
    ).toBe(true),
  );
  const list = await screen.findByRole("listbox", { name: "Messages" });
  await waitFor(() => expect(within(list).queryAllByRole("option").length).toBeGreaterThan(0));
  return { mock, client: new Client(mock), list };
}

/** flagCalls are the params of every message.setFlags the mock saw. */
function flagCalls(mock: MockTransport): unknown[] {
  return mock.calls.filter((c) => c.method === "message.setFlags").map((c) => c.params);
}

describe("Follow Up in the main window", () => {
  it("lists sent mail nobody answered, in its own section", async () => {
    const { calls } = setup([sent(9001, 9001, "Offsite plan", ANN, 4), sent(9002, 9002, "Signed contract", BOB, 5)]);
    const section = await screen.findByRole("region", { name: "Follow Up" });
    await waitFor(() => expect(within(section).getAllByRole("listitem")).toHaveLength(2));
    const rows = within(section).getAllByRole("listitem");
    expect(within(rows[0] as HTMLElement).getByText("Offsite plan")).toBeTruthy();
    expect(within(rows[0] as HTMLElement).getByText("To: Ann Smith")).toBeTruthy();
    expect(within(rows[0] as HTMLElement).getByText("Waiting 4 days")).toBeTruthy();
    expect(within(rows[1] as HTMLElement).getByText("To: Bob Okafor")).toBeTruthy();
    expect(within(section).queryByText(/^Showing /)).toBeNull();
    expect(calls()[0]).toEqual({ limit: 25, waitingDays: 3 });
  });

  it("shows nothing when every thread has an answer", async () => {
    setup([]);
    await screen.findByRole("listbox", { name: "Messages" });
    expect(screen.queryByRole("region", { name: "Follow Up" })).toBeNull();
  });

  it("opens a message where it lives, in Sent, and selects it", async () => {
    const { mock } = setup([sent(9001, 9001, "Offsite plan", ANN, 4)]);
    const section = await screen.findByRole("region", { name: "Follow Up" });
    fireEvent.click(await within(section).findByText("Offsite plan"));
    await waitFor(() => expect(useUI.getState().selected).toEqual([9001]));
    expect(useUI.getState().source).toEqual({ kind: "mailbox", mailboxId: FIXTURE.sent });
    expect(mock.calls.filter((c) => c.method === "message.summaries").map((c) => c.params)).toContainEqual({
      ids: [9001],
    });
  });

  it("clears a list filter that would hide the message", async () => {
    setup([sent(9001, 9001, "Offsite plan", ANN, 4)]);
    const section = await screen.findByRole("region", { name: "Follow Up" });
    act(() => useUI.getState().setListFilter("unread"));
    fireEvent.click(await within(section).findByText("Offsite plan"));
    await waitFor(() => expect(useUI.getState().selected).toEqual([9001]));
    expect(useUI.getState().listFilter).toBe("all");
    expect(useUI.getState().source).toEqual({ kind: "mailbox", mailboxId: FIXTURE.sent });
    await waitFor(() => expect(useUI.getState().revealing).toBeNull());
    expect(useUI.getState().selected).toEqual([9001]);
  });

  it("keeps a revealed message whose row is loaded, and clears the reveal", async () => {
    setup([sent(9001, 9001, "Offsite plan", ANN, 4)]);
    const section = await screen.findByRole("region", { name: "Follow Up" });
    fireEvent.click(await within(section).findByText("Offsite plan"));
    await waitFor(() => expect(useUI.getState().revealing).toBeNull());
    expect(useUI.getState().selected).toEqual([9001]);
    expect(useUI.getState().source).toEqual({ kind: "mailbox", mailboxId: FIXTURE.sent });
  });
});

describe("a pinned message the list has not loaded", () => {
  it("keeps it selected with its own summary", async () => {
    await pinned();
    expect(useUI.getState().source).toEqual({ kind: "mailbox", mailboxId: FIXTURE.sent });
    expect(useUI.getState().selected).toEqual([9001]);
    expect(useUI.getState().revealing?.id).toBe(9001);
    expect(useUI.getState().revealing?.subject).toBe("Offsite plan");
  });

  it("releases the pin when a Delete moves the message out of the source", async () => {
    const { client } = await pinned();
    await client.message.move({ ids: [9001], mailboxId: FIXTURE.trash });
    await waitFor(() => expect(useUI.getState().revealing).toBeNull());
    expect(useUI.getState().selected).not.toEqual([9001]);
  });

  it("releases the pin when the message is removed", async () => {
    const { mock } = await pinned();
    mock.remove([9001]);
    await waitFor(() => expect(useUI.getState().revealing).toBeNull());
  });

  it("marks it read, and flags it, as on a loaded row", async () => {
    const { client, mock } = await pinned();
    await toggleRead(client, null, [9001]);
    expect(flagCalls(mock).at(-1)).toEqual({ ids: [9001], changes: { seen: false } });
    await toggleFlag(client, null, [9001]);
    expect(flagCalls(mock).at(-1)).toEqual({ ids: [9001], changes: { flagColor: 1 } });
  });

  it("goes to the first row on the arrow keys", async () => {
    const { list } = await pinned();
    act(() => useUI.getState().setFocus("list"));
    fireEvent.keyDown(window, { key: "ArrowDown" });
    await waitFor(() => expect(useUI.getState().revealing).toBeNull());
    expect(useUI.getState().selected).toEqual([7000]);
    expect(within(list).getAllByRole("option")[0]?.getAttribute("aria-selected")).toBe("true");
  });
});
