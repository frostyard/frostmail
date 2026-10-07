// Display formatting for the reader UI (docs/specs/ui.md).
import type { Address } from "../rpc/gen/api";

/**
 * formatListDate shows a message list date by the local calendar days between
 * date and now: the short time for the same day, "Yesterday" for the day
 * before, the weekday for 2 to 6 days before, and the short numeric date
 * otherwise (including later days).
 */
export function formatListDate(date: Date, now: Date, locale?: string): string {
  const midnight = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
  const daysBefore = Math.round((midnight(now) - midnight(date)) / 86_400_000);
  if (daysBefore === 0) {
    return date.toLocaleTimeString(locale, { hour: "numeric", minute: "2-digit" });
  }
  if (daysBefore === 1) {
    return "Yesterday";
  }
  if (daysBefore >= 2 && daysBefore <= 6) {
    return date.toLocaleDateString(locale, { weekday: "long" });
  }
  return date.toLocaleDateString(locale, { year: "2-digit", month: "numeric", day: "numeric" });
}

/** formatHeaderDate formats a reader header date as a long date with a short time. */
export function formatHeaderDate(date: Date, locale?: string): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: "long", timeStyle: "short" }).format(date);
}

/**
 * formatSize formats a byte count with decimal units: "N bytes" below 1000
 * ("1 byte" for 1), then whole KB while under 1000 KB, then MB and from
 * 1000 MB GB, each rounded to one decimal with a trailing ".0" dropped.
 */
export function formatSize(bytes: number): string {
  if (bytes < 1000) {
    return `${bytes} ${bytes === 1 ? "byte" : "bytes"}`;
  }
  const kb = Math.round(bytes / 1000);
  if (kb < 1000) {
    return `${kb} KB`;
  }
  const mb = bytes / 1e6;
  if (mb < 1000) {
    return `${oneDecimal(mb)} MB`;
  }
  return `${oneDecimal(bytes / 1e9)} GB`;
}

/** oneDecimal rounds to one decimal place, dropping a trailing ".0". */
function oneDecimal(value: number): string {
  return String(Math.round(value * 10) / 10);
}

/** formatCount formats a count with grouping separators. */
export function formatCount(n: number, locale?: string): string {
  return n.toLocaleString(locale);
}

/**
 * displayName is what the UI shows for an address: the trimmed name, else
 * the trimmed address, else "Unknown Sender".
 */
export function displayName(a: Address): string {
  const name = a.name.trim();
  if (name !== "") {
    return name;
  }
  const address = a.address.trim();
  return address !== "" ? address : "Unknown Sender";
}

/**
 * formatAddressList joins display names with ", "; when the list is longer
 * than max, the first max followed by " & N more" for the rest.
 */
export function formatAddressList(list: Address[], max = 3): string {
  const names = list.map(displayName);
  if (list.length > max) {
    return `${names.slice(0, max).join(", ")} & ${list.length - max} more`;
  }
  return names.join(", ");
}

/**
 * initials are the letters on an avatar: the first letter of the first and
 * last words of the name (letters only), or of the address before "@" when
 * the name has none; "?" when there is nothing to use.
 */
export function initials(a: Address): string {
  const words = a.name
    .split(/\s+/)
    .map((word) => word.replace(/[^\p{L}]/gu, ""))
    .filter((word) => word.length > 0);
  if (words.length > 0) {
    const first = words[0]?.charAt(0) ?? "";
    const last = words.length >= 2 ? (words[words.length - 1]?.charAt(0) ?? "") : "";
    return (first + last).toUpperCase();
  }
  const local = a.address.split("@")[0]?.replace(/[^\p{L}]/gu, "") ?? "";
  return local.length > 0 ? local.charAt(0).toUpperCase() : "?";
}

/** avatarTone picks one of the eight avatar tones for an address by FNV-1a over its lowercased UTF-8 bytes. */
export function avatarTone(address: string): number {
  let hash = 0x811c9dc5;
  for (const byte of new TextEncoder().encode(address.toLowerCase())) {
    hash ^= byte;
    hash = Math.imul(hash, 0x01000193);
  }
  return (hash >>> 0) % 8;
}
