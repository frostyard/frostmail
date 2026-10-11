// CONTRACT TEST for task card T-0104 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import type { Account, Mailbox, ViewCount, Vip } from "../rpc/gen/api";
import { buildSidebar, type SidebarCounts, vipGroups } from "./mailboxTree";

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

const vips: Vip[] = [
  { address: "ann@x.test", name: "Ann Smith", personId: 7 },
  { address: "bob@y.test", name: "" },
  { address: "ann@home.test", name: "Ann Smith", personId: 7 },
  { address: "carol@z.test", name: "Carol" },
];

const count = (total: number, unread: number): ViewCount => ({ total, unread });

const counts: SidebarCounts = {
  vips: count(9, 4),
  vip: { "vip:ann@x.test": count(5, 3), "vip:carol@z.test": count(1, 1) },
  flagged: count(6, 2),
  colors: [count(4, 1), count(0, 0), count(2, 1), count(0, 0), count(0, 0), count(0, 0), count(0, 0)],
};

const favorites = (extras?: Parameters<typeof buildSidebar>[2]) => {
  const section = buildSidebar([account], [inbox], extras)[0];
  if (!section) throw new Error("no Favorites");
  return section.items;
};

describe("vipGroups", () => {
  it("makes one group per person, else per address, in the list's order", () => {
    expect(vipGroups(vips)).toEqual([
      { key: "vip:ann@x.test", label: "Ann Smith", addresses: ["ann@x.test", "ann@home.test"] },
      { key: "vip:bob@y.test", label: "bob@y.test", addresses: ["bob@y.test"] },
      { key: "vip:carol@z.test", label: "Carol", addresses: ["carol@z.test"] },
    ]);
    expect(vipGroups([])).toEqual([]);
  });
});

describe("Favorites' built-in sources", () => {
  it("adds VIPs with a row per group, and Flagged with a row per color in use", () => {
    const items = favorites({ vips, flagNames: [" Urgent ", "", "", "", "", "", ""], counts });
    expect(items.map((i) => i.key)).toEqual([
      "all-inboxes",
      "vips",
      "vip:ann@x.test",
      "vip:bob@y.test",
      "vip:carol@z.test",
      "flagged",
      "flag:1",
      "flag:3",
    ]);
    const by = (key: string) => {
      const item = items.find((i) => i.key === key);
      if (!item) throw new Error(`no ${key}`);
      return item;
    };
    expect(by("vips")).toMatchObject({ label: "VIPs", icon: "star", depth: 0, unread: 4, selectable: true });
    expect(by("vip:ann@x.test")).toMatchObject({ label: "Ann Smith", icon: "user", depth: 1, unread: 3 });
    expect(by("vip:bob@y.test")).toMatchObject({ label: "bob@y.test", icon: "user", depth: 1, unread: 0 });
    expect(by("flagged")).toMatchObject({ label: "Flagged", icon: "flag", depth: 0, unread: 6 });
    expect(by("flag:1")).toMatchObject({ label: "Urgent", icon: "flag", depth: 1, unread: 4, flagColor: 1 });
    expect(by("flag:3")).toMatchObject({ label: "Yellow", icon: "flag", depth: 1, unread: 2, flagColor: 3 });
    expect(by("all-inboxes").flagColor).toBeUndefined();
  });

  it("shows no VIPs without VIPs, and no colors before the counts", () => {
    const items = favorites();
    expect(items.map((i) => i.key)).toEqual(["all-inboxes", "flagged"]);
    expect(items[1]?.unread).toBe(0);
    expect(favorites({ vips: [], counts }).map((i) => i.key)).toEqual(["all-inboxes", "flagged", "flag:1", "flag:3"]);
  });
});
