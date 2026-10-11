// CONTRACT TEST for task card T-0119 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import type { Account } from "../rpc/gen/api";
import { buildSidebar } from "./mailboxTree";

const account = (id: number, readOnly: boolean): Account => ({
  id,
  kind: "imap",
  email: `me${id}@x.test`,
  displayName: "Me",
  auth: "password",
  imap: { host: "h", port: 993, tls: "tls", username: "me" },
  smtp: { host: "h", port: 465, tls: "tls", username: "me" },
  createdAt: "2026-10-01T00:00:00Z",
  readOnly,
  notify: true,
  syncDays: 0,
  signedIn: true,
});

describe("an account's New Mailbox", () => {
  it("is offered on a writable account's section", () => {
    const sections = buildSidebar([account(1, false), account(2, true)], []);
    expect(sections.map((s) => [s.key, s.addLabel])).toEqual([
      ["favorites", undefined],
      ["account:1", "New Mailbox"],
      ["account:2", undefined],
    ]);
  });
});
