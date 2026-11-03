import { afterEach, describe, expect, it } from "vitest";
import { useUI } from "../data/stores";
import type { Mailbox, MailboxRole, MessageSummary } from "../rpc/gen/api";
import { archiveOf, moveParams, selectedSummaries, sourceHolds } from "./commands";

function mb(id: number, accountId: number, path: string, role: MailboxRole, label = false): Mailbox {
  return { id, accountId, path, name: path, delimiter: "/", role, total: 0, unread: 0, label };
}

// Account 1 is IMAP with an Archive folder; account 2 is Gmail.
const work = mb(12, 2, "Work", "none", true);
const mailboxes = [
  mb(1, 1, "INBOX", "inbox"),
  mb(2, 1, "Archive", "archive"),
  mb(3, 1, "Projects", "none"),
  mb(10, 2, "INBOX", "inbox", true),
  mb(11, 2, "[Gmail]/All Mail", "all"),
  work,
  mb(20, 3, "INBOX", "inbox"),
  mb(21, 3, "All", "all"),
];

describe("archiveOf", () => {
  it("uses the archive mailbox", () => {
    expect(archiveOf(1, mailboxes)?.id).toBe(2);
  });
  it("uses All Mail on a Gmail account", () => {
    expect(archiveOf(2, mailboxes)?.id).toBe(11);
  });
  it("never archives into a virtual All folder of another server", () => {
    expect(archiveOf(3, mailboxes)).toBeUndefined();
  });
});

describe("moveParams", () => {
  it("leaves the mailbox the list shows", () => {
    expect(moveParams([5], work, { kind: "mailbox", mailboxId: 10 }, mailboxes)).toEqual({
      ids: [5],
      mailboxId: 12,
      fromMailboxId: 10,
    });
  });
  it("names no source for smart views or another account's mailbox", () => {
    expect(moveParams([5], work, { kind: "allInboxes" }, mailboxes)).toEqual({ ids: [5], mailboxId: 12 });
    expect(moveParams([5], work, { kind: "mailbox", mailboxId: 1 }, mailboxes)).toEqual({
      ids: [5],
      mailboxId: 12,
    });
  });
  it("leaves the account's mailbox of a unified source", () => {
    expect(moveParams([5], work, { kind: "role", role: "inbox" }, mailboxes)).toEqual({
      ids: [5],
      mailboxId: 12,
      fromMailboxId: 10,
    });
  });
});

function summary(id: number, over: Partial<MessageSummary> = {}): MessageSummary {
  return {
    id,
    accountId: 1,
    mailboxIds: [1],
    threadId: id,
    subject: `Message ${id}`,
    from: { name: "Test One", address: "test1@mailtest.test" },
    date: "2026-10-04T12:00:00Z",
    preview: "",
    flags: { seen: true, flagged: false, answered: false, forwarded: false, draft: false, flagColor: 0 },
    hasAttachments: false,
    size: 2000,
    threadCount: 1,
    ...over,
  };
}

afterEach(() => {
  useUI.setState({ revealing: null });
});

describe("sourceHolds", () => {
  it("holds a message in the source's mailbox", () => {
    expect(sourceHolds({ kind: "mailbox", mailboxId: 1 }, summary(1), mailboxes)).toBe(true);
    expect(sourceHolds({ kind: "mailbox", mailboxId: 3 }, summary(1), mailboxes)).toBe(false);
  });
  it("holds a message in an inbox for All Inboxes", () => {
    expect(sourceHolds({ kind: "allInboxes" }, summary(1), mailboxes)).toBe(true);
    expect(sourceHolds({ kind: "allInboxes" }, summary(1, { mailboxIds: [2] }), mailboxes)).toBe(false);
  });
  it("holds every message for a source that is not one mailbox", () => {
    expect(sourceHolds({ kind: "flagged" }, summary(1, { mailboxIds: [2] }), mailboxes)).toBe(true);
  });
});

describe("selectedSummaries", () => {
  it("uses the pinned summary of a message the list has not loaded", () => {
    useUI.setState({ revealing: summary(7, { subject: "Pinned" }) });
    expect(selectedSummaries(null, [7]).map((s) => s.subject)).toEqual(["Pinned"]);
    expect(selectedSummaries(null, [8])).toEqual([]);
  });
});
