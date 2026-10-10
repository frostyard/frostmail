// CONTRACT TEST for task card T-0103 (docs/tasks). Do not edit.
import { describe, expect, it, vi } from "vitest";

import type { ViewModel } from "../data/view";
import type { Client, Mailbox, MailboxRole, MessageSummary } from "../rpc/gen/api";
import { copyMessages, copyTargets, junkOf, moveTargets, spamTarget, toggleSpam } from "./commands";

function mb(id: number, accountId: number, path: string, role: MailboxRole, label = false): Mailbox {
  return { id, accountId, path, name: path, delimiter: "/", role, total: 0, unread: 0, label };
}

// Account 1 is IMAP; account 2 is Gmail; account 3 has no junk mailbox.
const mailboxes = [
  mb(1, 1, "INBOX", "inbox"),
  mb(2, 1, "Junk", "junk"),
  mb(3, 1, "Archive", "archive"),
  mb(4, 1, "Projects", "none"),
  mb(10, 2, "INBOX", "inbox", true),
  mb(11, 2, "[Gmail]/All Mail", "all"),
  mb(12, 2, "[Gmail]/Spam", "junk"),
  mb(13, 2, "[Gmail]/Starred", "flagged", true),
  mb(14, 2, "Work", "none", true),
  mb(15, 2, "[Gmail]/Trash", "trash"),
  mb(20, 3, "INBOX", "inbox"),
];

function summary(id: number, accountId: number, mailboxIds: number[]): MessageSummary {
  return {
    id,
    accountId,
    mailboxIds,
    threadId: 0,
    subject: "",
    from: { name: "", address: "a@b.test" },
    date: "2026-10-08T12:00:00Z",
    preview: "",
    flags: { seen: true, flagged: false, answered: false, forwarded: false, draft: false, flagColor: 0 },
    hasAttachments: false,
    size: 0,
    threadCount: 1,
  };
}

function model(rows: MessageSummary[]): ViewModel {
  return {
    indexOf: (id: number) => rows.findIndex((r) => r.id === id),
    row: (i: number) => rows[i],
  } as unknown as ViewModel;
}

const inInbox = summary(100, 1, [1]);
const inJunk = summary(101, 1, [2]);
const alsoJunk = summary(102, 1, [2]);
const gmailInbox = summary(200, 2, [10, 11]);
const gmailSpam = summary(201, 2, [12]);
const noJunk = summary(300, 3, [20]);
const rows = model([inInbox, inJunk, alsoJunk, gmailInbox, gmailSpam, noJunk]);

describe("junkOf and spamTarget", () => {
  it("finds an account's junk mailbox", () => {
    expect(junkOf(1, mailboxes)?.id).toBe(2);
    expect(junkOf(2, mailboxes)?.id).toBe(12);
    expect(junkOf(3, mailboxes)).toBeUndefined();
  });

  it("marks as spam into junk", () => {
    expect(spamTarget(rows, [100], mailboxes)).toEqual({ notSpam: false, to: mailboxes[1] });
    expect(spamTarget(rows, [100, 101], mailboxes)).toEqual({ notSpam: false, to: mailboxes[1] });
    expect(spamTarget(rows, [200], mailboxes).to?.id).toBe(12);
  });

  it("is Not Spam, back to the inbox, when every message is in junk", () => {
    expect(spamTarget(rows, [101, 102], mailboxes)).toEqual({ notSpam: true, to: mailboxes[0] });
    expect(spamTarget(rows, [201], mailboxes)).toEqual({ notSpam: true, to: mailboxes[4] });
  });

  it("has nowhere to go without a junk mailbox or messages", () => {
    expect(spamTarget(rows, [300], mailboxes)).toEqual({ notSpam: false, to: undefined });
    expect(spamTarget(rows, [], mailboxes)).toEqual({ notSpam: false, to: undefined });
    expect(spamTarget(null, [100], mailboxes)).toEqual({ notSpam: false, to: undefined });
  });

  it("moves through message.move", async () => {
    const move = vi.fn().mockResolvedValue(undefined);
    const client = { message: { move } } as unknown as Client;
    await toggleSpam(client, rows, [100], { kind: "mailbox", mailboxId: 1 }, mailboxes);
    await toggleSpam(client, rows, [101], { kind: "mailbox", mailboxId: 2 }, mailboxes);
    await toggleSpam(client, rows, [300], { kind: "allInboxes" }, mailboxes);
    expect(move.mock.calls).toEqual([
      [{ ids: [100], mailboxId: 2, fromMailboxId: 1 }],
      [{ ids: [101], mailboxId: 1, fromMailboxId: 2 }],
    ]);
  });
});

describe("moveTargets and copyTargets", () => {
  it("offer the account's other mailboxes", () => {
    expect(moveTargets(inInbox, mailboxes).map((m) => m.id)).toEqual([2, 3, 4]);
    expect(copyTargets(inInbox, mailboxes).map((m) => m.id)).toEqual([2, 3, 4]);
    expect(moveTargets(undefined, mailboxes)).toEqual([]);
    expect(copyTargets(undefined, mailboxes)).toEqual([]);
  });

  it("copy only into labels on Gmail, and not into Starred", () => {
    expect(moveTargets(gmailInbox, mailboxes).map((m) => m.id)).toEqual([12, 13, 14, 15]);
    expect(copyTargets(gmailInbox, mailboxes).map((m) => m.id)).toEqual([14]);
  });
});

describe("copyMessages", () => {
  it("calls message.copy", async () => {
    const copy = vi.fn().mockResolvedValue(undefined);
    const client = { message: { copy } } as unknown as Client;
    await copyMessages(client, [100, 101], mailboxes[3]);
    await copyMessages(client, [], mailboxes[3]);
    await copyMessages(client, [100], undefined);
    expect(copy.mock.calls).toEqual([[{ ids: [100, 101], mailboxId: 4 }]]);
  });
});
