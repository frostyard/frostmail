// Display names and shared classes of the settings window
// (docs/specs/settings-ui.md).
import type { AccountKind, TLSMode } from "../../rpc/gen/api";

/** KIND_LABEL names each account kind. */
export const KIND_LABEL: Record<AccountKind, string> = {
  imap: "IMAP",
  gmail: "Gmail",
  microsoft: "Microsoft",
  icloud: "iCloud",
};

/** TLS_MODES are the security modes in the order offered. */
export const TLS_MODES: readonly TLSMode[] = ["tls", "starttls", "insecure"];

/** TLS_LABEL names each security mode. */
export const TLS_LABEL: Record<TLSMode, string> = {
  tls: "TLS",
  starttls: "STARTTLS",
  insecure: "None (insecure)",
};

/** GRID lays out labeled rows: labels in a 128px column, controls after. */
export const GRID = "grid grid-cols-[128px_1fr] items-center gap-x-3 gap-y-2";

/** LABEL is a field label. */
export const LABEL = "text-right text-[13px] leading-[18px] text-secondary";

/** FIELD is a text input or select. */
export const FIELD =
  "h-7 rounded-md border border-separator bg-window px-2 text-[13px] leading-[18px] focus:outline-none focus:ring-2 focus:ring-focus";

/** BUTTON is a secondary push button. */
export const BUTTON =
  "h-7 rounded-md border border-separator bg-window px-3 text-[13px] leading-[18px] disabled:opacity-40";

/** PRIMARY_BUTTON is a form's default button. */
export const PRIMARY_BUTTON =
  "h-7 rounded-md bg-accent px-3 text-[13px] leading-[18px] font-semibold text-accent-contrast disabled:opacity-40";

/** ALERT shows a request's error. */
export const ALERT = "mt-4 text-[12px] leading-4 text-flag-1";
