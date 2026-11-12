// The reminder window: raising it from the main window, and asking the main
// window to open an occurrence (docs/specs/pim-ui.md, Reminder window). In
// lodge every window is a route the frame shows, in the same document.
import { useUI } from "../data/stores";
import { closeFrameWindow, frameWindowOpen, openFrameWindow } from "../lodge/frameWindow";
import type { Client } from "../rpc/gen/api";

/** OccurrenceRequest names an occurrence for the main window to open. */
export interface OccurrenceRequest {
  eventId: number;
  recurrenceId: string;
  /** Its date in the app's zone, YYYY-MM-DD. */
  date: string;
}

/** openReminders shows the reminder window, raising it when it is open. */
export async function openReminders(): Promise<void> {
  // The reminders never cover a window the user is in.
  if (!frameWindowOpen()) openFrameWindow({ kind: "reminders" });
}

/** openOccurrenceInMain asks the main window to open an occurrence in Calendar. */
export async function openOccurrenceInMain(request: OccurrenceRequest): Promise<void> {
  closeFrameWindow(); // back to the main window to show it
  window.dispatchEvent(new CustomEvent<OccurrenceRequest>("open-occurrence", { detail: request }));
}

/** watchReminders raises the reminder window from the main window; it returns the cleanup. */
export function watchReminders(client: Client): () => void {
  let stopped = false;
  const raise = () => {
    if (!stopped) void openReminders().catch((err: unknown) => console.warn("open reminders", err));
  };
  const off = client.transport.onEvent((event) => {
    if (event.event === "calendar.reminders" && event.data.count > 0) raise();
  });
  void client.calendar
    .reminders({})
    .then((rows) => {
      if (rows.length > 0) raise();
    })
    .catch((err: unknown) => console.warn("reminders", err));
  return () => {
    stopped = true;
    off();
  };
}

/** watchOccurrenceRequests opens the occurrences other windows ask for; it returns the cleanup. */
export function watchOccurrenceRequests(): () => void {
  let stopped = false;
  const reveal = (request: OccurrenceRequest) => {
    if (stopped) return;
    const ui = useUI.getState();
    ui.setModule("calendar");
    ui.selectOccurrence({ eventId: request.eventId, recurrenceId: request.recurrenceId }, request.date);
  };
  const onLocalRequest = (event: Event) => {
    if (event instanceof CustomEvent) reveal(event.detail as OccurrenceRequest);
  };
  window.addEventListener("open-occurrence", onLocalRequest);
  return () => {
    stopped = true;
    window.removeEventListener("open-occurrence", onLocalRequest);
  };
}
