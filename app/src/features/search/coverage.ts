// How far back a search looked: the store holds only each account's sync
// window (docs/specs/ui.md, Search coverage; ADR-0016, the sync window).
import type { Account } from "../../rpc/gen/api";

/** searchedAccounts are the accounts a search covers: the account of
 * accountId, or every account when it is undefined. */
export function searchedAccounts(accounts: Account[], accountId: number | undefined): Account[] {
  if (accountId === undefined) return accounts;
  return accounts.filter((account) => account.id === accountId);
}

/** firstDay is the window's first day in maild's terms: the UTC date
 * `days` days before now (internal/mailsync/window.go), in the long date
 * form of locale. */
function firstDay(days: number, now: Date, locale?: string): string {
  const start = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate() - days));
  return new Intl.DateTimeFormat(locale, { dateStyle: "long", timeZone: "UTC" }).format(start);
}

/** accountName is what the line calls an account: its display name, or its
 * address when the name is empty. */
function accountName(account: Account): string {
  const name = account.displayName.trim();
  return name === "" ? account.email : name;
}

/**
 * searchCoverage says how far back the searched accounts' stored mail goes,
 * or null when every one of them keeps every message. One window shared by
 * every searched account gets the short form; otherwise each account with a
 * window is named, in the given order.
 */
export function searchCoverage(accounts: Account[], now: Date, locale?: string): string | null {
  const windowed = accounts.filter((account) => account.syncDays > 0);
  if (windowed.length === 0) return null;
  const keepsEverything = accounts.some((account) => account.syncDays === 0);
  const shared = windowed.every((account) => account.syncDays === windowed[0]?.syncDays);
  if (shared && !keepsEverything) {
    return `Searched mail since ${firstDay(windowed[0]?.syncDays ?? 0, now, locale)} on this computer.`;
  }
  const named = windowed.map((account) => `${accountName(account)}: since ${firstDay(account.syncDays, now, locale)}`);
  return `Searched mail on this computer. ${named.join("; ")}.`;
}
