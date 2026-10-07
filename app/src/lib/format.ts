// Display formatting for the reader UI (docs/specs/ui.md). Task T-0026
// implements every function here; the stubs return empty values.
import type { Address } from "../rpc/gen/api";

/** formatListDate formats a message list date. */
export function formatListDate(_date: Date, _now: Date, _locale?: string): string {
  return "";
}

/** formatHeaderDate formats a reader header date. */
export function formatHeaderDate(_date: Date, _locale?: string): string {
  return "";
}

/** formatSize formats a byte count. */
export function formatSize(_bytes: number): string {
  return "";
}

/** formatCount formats a count with grouping separators. */
export function formatCount(_n: number, _locale?: string): string {
  return "";
}

/** displayName is what the UI shows for an address. */
export function displayName(_a: Address): string {
  return "";
}

/** formatAddressList joins addresses for a header line. */
export function formatAddressList(_list: Address[], _max?: number): string {
  return "";
}

/** initials are the letters on an avatar. */
export function initials(_a: Address): string {
  return "";
}

/** avatarTone picks one of the eight avatar tones for an address. */
export function avatarTone(_address: string): number {
  return 0;
}
