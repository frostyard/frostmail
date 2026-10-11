// CONTRACT TEST for task card T-0108 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import type { Account, Mailbox, SmartMailbox } from "../rpc/gen/api";
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

const inbox: Mailbox = {
  id: 10,
  accountId: 1,
  path: "INBOX",
  name: "INBOX",
  delimiter: "/",
  role: "inbox",
  total: 5,
  unread: 2,
  label: false,
};

const smart = (id: number, name: string): SmartMailbox => ({
  id,
  name,
  position: id - 1,
  conditions: { match: "all", conditions: [] },
  includeTrash: false,
  includeSent: false,
  unread: 99,
});

describe("the Smart Mailboxes section", () => {
  it("follows Favorites, a row per smart mailbox with its unread count", () => {
    const sections = buildSidebar([account], [inbox], {
      smarts: [smart(1, "Today"), smart(2, "From Ann")],
      smartCounts: { 1: { total: 4, unread: 3 } },
    });
    expect(sections.map((s) => s.key)).toEqual(["favorites", "smart", "account:1"]);
    const section = sections[1];
    expect(section?.title).toBe("Smart Mailboxes");
    expect(section?.addLabel).toBe("New Smart Mailbox");
    expect(section?.items).toEqual([
      { key: "smart:1", label: "Today", icon: "folder-cog", depth: 0, unread: 3, selectable: true },
      { key: "smart:2", label: "From Ann", icon: "folder-cog", depth: 0, unread: 0, selectable: true },
    ]);
  });

  it("is there, empty, when there are none, and only when asked for", () => {
    const sections = buildSidebar([account], [inbox], { smarts: [] });
    expect(sections.map((s) => s.key)).toEqual(["favorites", "smart", "account:1"]);
    expect(sections[1]?.items).toEqual([]);
    expect(sections[0]?.addLabel).toBeUndefined();
    expect(buildSidebar([account], [inbox]).map((s) => s.key)).toEqual(["favorites", "account:1"]);
  });
});
