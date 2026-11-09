// A conversation shows read, unselected messages collapsed to 1 row each
// (docs/specs/ui.md, Reader, Conversation; parity P-310).
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { formatHeaderDate } from "../lib/format";
import type { MessageSummary } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { ReaderContainer } from "./ReaderContainer";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

/**
 * setup shows the reader alone on the fixture: the list's selection rules
 * would move a selection of a message that is not a row (the Sent reply).
 * The fixture's first thread is
 * "Offsite plan": Ann (read), my reply in Sent (read), and Ann's newest
 * reply (unread unless allRead).
 */
async function setup(allRead: boolean) {
  const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") });
  const offsite = data.messages.map((m) => m.summary).filter((s) => s.threadId === data.messages[0]?.summary.threadId);
  expect(offsite).toHaveLength(3);
  if (allRead) for (const s of offsite) s.flags.seen = true;
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <ReaderContainer />
    </Session>,
  );
  const reader = await screen.findByRole("region", { name: "Message" });
  const gets = () => mock.calls.filter((c) => c.method === "message.get").map((c) => (c.params as { id: number }).id);
  const [first, middle, newest] = offsite as [MessageSummary, MessageSummary, MessageSummary];
  return { data, mock, reader, gets, first, middle, newest };
}

function open(id: number) {
  act(() => useUI.getState().select([id], id));
}

/** rowName is a collapsed row's accessible name: the sender and the long date. */
function rowName(s: MessageSummary): string {
  return `${s.from.name}, ${formatHeaderDate(new Date(s.date))}`;
}

/** expandedArticles waits for n expanded messages, each with its header. */
async function expandedArticles(reader: HTMLElement, n: number): Promise<HTMLElement[]> {
  return waitFor(() => {
    const articles = within(reader).queryAllByRole("article");
    expect(articles).toHaveLength(n);
    for (const a of articles) expect(a.querySelector("header")).not.toBeNull();
    return articles;
  });
}

describe("a conversation's collapsed messages", () => {
  it("expands only the selected message when every message is read", async () => {
    const { reader, gets, first, middle, newest } = await setup(true);
    open(middle.id);
    const [article] = await expandedArticles(reader, 1);
    expect(within(article as HTMLElement).getByText("Lisbon works for me.", { exact: false })).toBeTruthy();
    for (const s of [first, newest]) {
      const row = within(reader).getByRole("button", { name: rowName(s) });
      expect(row.getAttribute("aria-expanded")).toBe("false");
      expect(row.textContent).toContain("Ann Smith");
      expect(row.textContent).toContain(s.preview);
    }
    // The rows stay in the conversation's order, newest at the top.
    const rows = within(reader).getAllByRole("button", { name: /^Ann Smith, / });
    expect(rows.map((r) => r.getAttribute("aria-label"))).toEqual([rowName(newest), rowName(first)]);
    expect(gets()).not.toContain(first.id);
    expect(gets()).not.toContain(newest.id);
  });

  it("opens an unread message expanded", async () => {
    const { reader, first, middle, newest } = await setup(false);
    open(first.id);
    const articles = await expandedArticles(reader, 2);
    expect(articles.map((a) => within(a).getByRole("button", { name: "ann.smith@northwind.test" }))).toHaveLength(2);
    expect(within(reader).getByRole("button", { name: rowName(middle) })).toBeTruthy();
    expect(within(reader).queryByRole("button", { name: rowName(newest) })).toBeNull();
    expect(within(reader).queryByRole("button", { name: rowName(first) })).toBeNull();
  });

  it("calls message.get for a collapsed message only when a click expands it", async () => {
    const { reader, gets, first, newest } = await setup(true);
    open(newest.id);
    await expandedArticles(reader, 1);
    const row = within(reader).getByRole("button", { name: rowName(first) });
    expect(gets()).not.toContain(first.id);
    fireEvent.click(row);
    await expandedArticles(reader, 2);
    expect(gets()).toContain(first.id);
    expect(within(reader).queryByRole("button", { name: rowName(first) })).toBeNull();
  });

  it("expands a focused row with Enter and with Space", async () => {
    const user = userEvent.setup();
    const { reader, middle, newest, first } = await setup(true);
    open(middle.id);
    await expandedArticles(reader, 1);
    const top = within(reader).getByRole("button", { name: rowName(newest) });
    top.focus();
    await user.keyboard("{Enter}");
    await expandedArticles(reader, 2);
    const bottom = within(reader).getByRole("button", { name: rowName(first) });
    bottom.focus();
    await user.keyboard(" ");
    await expandedArticles(reader, 3);
  });

  it("collapses an expanded message on a click in its header, not on an address in it", async () => {
    const { reader, gets, middle, newest } = await setup(true);
    open(newest.id);
    const [article] = (await expandedArticles(reader, 1)) as [HTMLElement];
    fireEvent.click(within(article).getByRole("button", { name: "test1@mailtest.test" }));
    await expandedArticles(reader, 1);
    expect(within(reader).queryByRole("button", { name: rowName(newest) })).toBeNull();
    fireEvent.click(within(article).getByText(newest.subject, { selector: "header *" }));
    await waitFor(() => expect(within(reader).queryAllByRole("article")).toHaveLength(0));
    const row = within(reader).getByRole("button", { name: rowName(newest) });
    expect(row.getAttribute("aria-expanded")).toBe("false");
    // The middle message (mine, in Sent) stays a row, unloaded.
    expect(
      within(reader).getByRole("button", { name: new RegExp(`, ${formatHeaderDate(new Date(middle.date))}$`) }),
    ).toBeTruthy();
    expect(gets()).not.toContain(middle.id);
  });

  it("shows a conversation of 1 message expanded, with no row", async () => {
    const { data, reader } = await setup(true);
    const single = data.messages.find((m) => m.summary.subject === "Signed contract and invoice")?.summary;
    if (!single) throw new Error("no single message");
    open(single.id);
    const [article] = (await expandedArticles(reader, 1)) as [HTMLElement];
    expect(within(article).getByText("Signed contract and invoice")).toBeTruthy();
    expect(within(reader).queryByRole("button", { name: rowName(single) })).toBeNull();
    expect(reader.querySelectorAll('button[aria-expanded="false"]')).toHaveLength(0);
    // A click on the header of the only message does not collapse it.
    fireEvent.click(within(article).getByText("Signed contract and invoice"));
    await expandedArticles(reader, 1);
  });
});
