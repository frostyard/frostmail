// CONTRACT TEST for task card T-0027 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import type { Account, Mailbox, MailboxRole } from "../rpc/gen/api";
import { buildSidebar, type SidebarItem } from "./mailboxTree";

function account(id: number, email: string): Account {
  const server = { host: "h", port: 993, tls: "tls" as const, username: email };
  return {
    id,
    kind: "imap",
    email,
    displayName: "Someone",
    auth: "password",
    imap: server,
    smtp: server,
    createdAt: "2026-10-07T00:00:00Z",
    readOnly: false,
    notify: true,
    signedIn: true,
  };
}

let nextId = 1;
function mb(accountId: number, path: string, role: MailboxRole, unread = 0, delimiter = "/"): Mailbox {
  const name = delimiter ? (path.split(delimiter).pop() ?? path) : path;
  return { id: nextId++, accountId, path, name, delimiter, role, total: unread * 2, unread };
}

const rows = (items: SidebarItem[]) =>
  items.map((i) => [
    i.key.startsWith("mailbox:") ? "mailbox" : i.key,
    i.label,
    i.icon,
    i.depth,
    i.unread,
    i.selectable,
  ]);

describe("buildSidebar", () => {
  it("always has Favorites, even without accounts", () => {
    const s = buildSidebar([], []);
    expect(s).toHaveLength(1);
    expect(s[0]?.key).toBe("favorites");
    expect(s[0]?.title).toBe("Favorites");
    expect(s[0]?.accountId).toBeUndefined();
    expect(rows(s[0]?.items ?? [])).toEqual([
      ["all-inboxes", "All Inboxes", "inbox", 0, 0, true],
      ["flagged", "Flagged", "flag", 0, 0, true],
    ]);
  });

  it("orders role mailboxes first, then folders by name, nested by path", () => {
    nextId = 1;
    const list = [
      mb(1, "Projects/Ice", "none"),
      mb(1, "Trash", "trash"),
      mb(1, "Projects", "none", 2),
      mb(1, "INBOX", "inbox", 12),
      mb(1, "Sent Messages", "sent"),
      mb(1, "Travel/2026", "none", 1),
      mb(1, "a-lower", "none"),
      mb(1, "Projects/Frost", "none", 3),
      mb(1, "Junk", "junk", 4),
      mb(1, "Drafts", "drafts"),
      mb(1, "Archive", "archive"),
    ];
    const s = buildSidebar([account(1, "test1@mailtest.test")], list);
    expect(s.map((x) => x.key)).toEqual(["favorites", "account:1"]);
    const acct = s[1];
    expect(acct?.title).toBe("test1@mailtest.test");
    expect(acct?.accountId).toBe(1);
    expect(rows(acct?.items ?? [])).toEqual([
      ["mailbox", "Inbox", "inbox", 0, 12, true],
      ["mailbox", "Drafts", "file", 0, 0, true],
      ["mailbox", "Sent", "send", 0, 0, true],
      ["mailbox", "Junk", "shield-alert", 0, 4, true],
      ["mailbox", "Trash", "trash-2", 0, 0, true],
      ["mailbox", "Archive", "archive", 0, 0, true],
      ["mailbox", "a-lower", "folder", 0, 0, true],
      ["mailbox", "Projects", "folder", 0, 2, true],
      ["mailbox", "Frost", "folder", 1, 3, true],
      ["mailbox", "Ice", "folder", 1, 0, true],
      ["path:1:Travel", "Travel", "folder", 0, 0, false],
      ["mailbox", "2026", "folder", 1, 1, true],
    ]);
    // Every row of a listed mailbox names it, by key and by mailboxId.
    for (const item of acct?.items ?? []) {
      if (!item.selectable) continue;
      const box = list.find((m) => m.id === item.mailboxId);
      expect(item.key).toBe(`mailbox:${box?.id}`);
      expect(box?.name === item.label || box?.role !== "none").toBe(true);
    }
    expect(list.find((m) => m.id === acct?.items[0]?.mailboxId)?.path).toBe("INBOX");
    expect(list.find((m) => m.id === acct?.items[8]?.mailboxId)?.path).toBe("Projects/Frost");
    expect(acct?.items[10]?.mailboxId).toBeUndefined();
    expect(rows(s[0]?.items ?? [])[0]).toEqual(["all-inboxes", "All Inboxes", "inbox", 0, 12, true]);
  });

  it("keeps role mailboxes top-level under an INBOX namespace and nests folders under Inbox", () => {
    nextId = 100;
    const list = [
      mb(2, "INBOX", "inbox", 1, "."),
      mb(2, "INBOX.Sent", "sent", 0, "."),
      mb(2, "INBOX.Receipts", "none", 5, "."),
      mb(2, "INBOX.Receipts.2025", "none", 0, "."),
    ];
    const s = buildSidebar([account(2, "b@mailtest.test")], list);
    expect(rows(s[1]?.items ?? [])).toEqual([
      ["mailbox", "Inbox", "inbox", 0, 1, true],
      ["mailbox", "Receipts", "folder", 1, 5, true],
      ["mailbox", "2025", "folder", 2, 0, true],
      ["mailbox", "Sent", "send", 0, 0, true],
    ]);
  });

  it("makes one section per account in the order given and sums every inbox", () => {
    nextId = 200;
    const list = [mb(3, "INBOX", "inbox", 2), mb(4, "INBOX", "inbox", 5), mb(4, "Notes", "none", 0, "")];
    const s = buildSidebar([account(4, "d@x.test"), account(3, "c@x.test")], list);
    expect(s.map((x) => x.title)).toEqual(["Favorites", "d@x.test", "c@x.test"]);
    expect(s[0]?.items[0]?.unread).toBe(7);
    expect(rows(s[1]?.items ?? [])).toEqual([
      ["mailbox", "Inbox", "inbox", 0, 5, true],
      ["mailbox", "Notes", "folder", 0, 0, true],
    ]);
  });
});
