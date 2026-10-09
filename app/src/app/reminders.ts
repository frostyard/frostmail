// The reminder window: raising it from the main window, and asking the main
// window to open an occurrence (docs/specs/pim-ui.md, Reminder window).
// Task T-0076 writes these.
import type { Client } from "../rpc/gen/api";

declare global {
  interface Window {
    /** Set by the reminder window's initialization script. */
    __frostmailReminders?: boolean;
  }
}

/** OccurrenceRequest names an occurrence for the main window to open. */
export interface OccurrenceRequest {
  eventId: number;
  recurrenceId: string;
  /** Its date in the app's zone, YYYY-MM-DD. */
  date: string;
}

/** isRemindersWindow reports whether this window shows the reminders. */
export function isRemindersWindow(): boolean {
  return window.__frostmailReminders === true || window.location.hash === "#/reminders";
}

/** openReminders shows the reminder window, raising it when it is open. */
export async function openReminders(): Promise<void> {}

/** openOccurrenceInMain asks the main window to open an occurrence in Calendar. */
export async function openOccurrenceInMain(_request: OccurrenceRequest): Promise<void> {}

/** watchReminders raises the reminder window from the main window; it returns the cleanup. */
export function watchReminders(_client: Client): () => void {
  return () => {};
}

/** watchOccurrenceRequests opens the occurrences other windows ask for; it returns the cleanup. */
export function watchOccurrenceRequests(): () => void {
  return () => {};
}
