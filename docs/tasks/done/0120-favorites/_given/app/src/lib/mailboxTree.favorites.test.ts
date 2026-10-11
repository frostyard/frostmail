// CONTRACT TEST for task card T-0120 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { sourceFromKey } from "../data/stores";
import type { Account, Mailbox, MailboxRole } from "../rpc/gen/api";
import { buildSidebar } from "./mailboxTree";

const account: Account = {
  id: 1,
  kind: "imap",
  email: "me@x.test",
  displayName: "Me",
  auth: "password",
  imap: { host: "h", port: 993, tls: "tls", username: "me" },
  smtp: { host: "h", port: 465, tls: "tls", username: "me" },
  createdAt: "2026-10-01T00:00:00Z",
  readOnly: false,
  notify: true,
  syncDays: 0,
  signedIn: true,
};

const mb = (id: number, path: string, role: MailboxRole, unread: number): Mailbox => ({
  id,
  accountId: 1,
  path,
  name: path.split("/").pop() ?? path,
  delimiter: "/",
  role,
  total: unread + 3,
  unread,
  label: false,
});

describe("Favorites", () => {
  it("lists the favorite mailboxes in order after the built-in rows", () => {
    const inbox = mb(10, "INBOX", "inbox", 2);
    const frost = mb(11, "Projects/Frost", "none", 4);
    const archive = mb(12, "Archive", "archive", 0);
    const [favorites] = buildSidebar([account], [inbox, frost, archive], { favorites: [frost, archive] });
    const rows = favorites?.items.map((i) => [i.key, i.label, i.icon, i.depth, i.unread, i.selectable]);
    expect(rows).toEqual([
      ["all-inboxes", "All Inboxes", "inbox", 0, 2, true],
      ["flagged", "Flagged", "flag", 0, 0, true],
      ["favorite:11", "Frost", "folder", 0, 4, true],
      ["favorite:12", "Archive", "archive", 0, 0, true],
    ]);
  });

  it("shows the favorite's mailbox when chosen", () => {
    expect(sourceFromKey("favorite:11")).toEqual({ kind: "mailbox", mailboxId: 11 });
  });
});
