// The reminder window's content (docs/specs/pim-ui.md, Reminder window).
// Task T-0076 builds it.
import type { Reminder } from "../../rpc/gen/api";

/** ReminderPanelProps are the reminder window's inputs. */
export interface ReminderPanelProps {
  reminders: Reminder[];
  /** Each calendar's color: #rrggbb, or "" for the accent color. */
  colors: ReadonlyMap<number, string>;
  timeZone: string;
  locale: string;
  now: Date;
  onSnooze: (id: string, until: Date) => void;
  onDismiss: (ids: string[]) => void;
  onOpen: (reminder: Reminder) => void;
  onClose: () => void;
}

/** ReminderPanel shows the title strip, the due reminders and Dismiss All. */
export function ReminderPanel(_props: ReminderPanelProps) {
  return null;
}
