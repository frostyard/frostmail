// How far back a search looked (docs/specs/ui.md, Search coverage; ADR-0016).
import { describe, expect, it } from "vitest";

import type { Account } from "../../rpc/gen/api";
import { searchCoverage, searchedAccounts } from "./coverage";

const server = { host: "h", port: 993, tls: "tls" as const, username: "u" };
const now = new Date("2026-08-07T12:00:00Z");

function account(over: Partial<Account>): Account {
  return {
    id: 1,
    kind: "imap",
    email: "ann@x.test",
    displayName: "Ann Example",
    auth: "password",
    imap: server,
    smtp: server,
    createdAt: "2026-10-01T00:00:00Z",
    readOnly: false,
    notify: true,
    syncDays: 0,
    signedIn: true,
    ...over,
  };
}

describe("searchCoverage", () => {
  it("gives the short form for one account's window", () => {
    expect(searchCoverage([account({ syncDays: 365 })], now, "en-US")).toBe(
      "Searched mail since August 7, 2025 on this computer.",
    );
  });

  it("gives the short form when every account shares one window", () => {
    const accounts = [account({ id: 1, syncDays: 365 }), account({ id: 2, displayName: "Work", syncDays: 365 })];
    expect(searchCoverage(accounts, now, "en-US")).toBe("Searched mail since August 7, 2025 on this computer.");
  });

  it("names the windowed account when another keeps every message", () => {
    const accounts = [account({ id: 1, displayName: "Gmail", syncDays: 365 }), account({ id: 2, syncDays: 0 })];
    expect(searchCoverage(accounts, now, "en-US")).toBe("Searched mail on this computer. Gmail: since August 7, 2025.");
  });

  it("names each account with its own window, in the given order", () => {
    const accounts = [
      account({ id: 1, displayName: "Ann Example", syncDays: 365 }),
      account({ id: 2, displayName: "  ", email: "work@x.test", syncDays: 30 }),
    ];
    expect(searchCoverage(accounts, now, "en-US")).toBe(
      "Searched mail on this computer. Ann Example: since August 7, 2025; work@x.test: since July 8, 2026.",
    );
  });

  it("says nothing when every account keeps every message", () => {
    expect(searchCoverage([account({ id: 1 }), account({ id: 2, syncDays: 0 })], now, "en-US")).toBeNull();
  });

  it("says nothing without accounts", () => {
    expect(searchCoverage([], now, "en-US")).toBeNull();
  });
});

describe("searchedAccounts", () => {
  it("is every account without an account id", () => {
    const accounts = [account({ id: 1 }), account({ id: 2 })];
    expect(searchedAccounts(accounts, undefined)).toEqual(accounts);
  });

  it("is only the account named", () => {
    const accounts = [account({ id: 1 }), account({ id: 2 })];
    expect(searchedAccounts(accounts, 2).map((a) => a.id)).toEqual([2]);
  });
});
