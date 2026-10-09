// The Calendar module's sidebar: the small month and each account's
// calendars (docs/specs/pim-ui.md, Calendar sidebar). Task T-0071 builds it.
import type { MiniMonthProps } from "./MiniMonth";

/** CalendarRow is one calendar in the sidebar. */
export interface CalendarRow {
  id: number;
  name: string;
  /** #rrggbb, or "" for the accent color. */
  color: string;
  /** Shown (and synced). */
  enabled: boolean;
  readOnly: boolean;
}

/** CalendarSection is an account and its calendars. */
export interface CalendarSection {
  accountId: number;
  title: string;
  calendars: CalendarRow[];
}

/** CalendarSidebarProps are the Calendar sidebar's inputs. */
export interface CalendarSidebarProps extends MiniMonthProps {
  sections: CalendarSection[];
  onToggle: (id: number, enabled: boolean) => void;
}

/** CalendarSidebar shows the small month and the calendars to show. */
export function CalendarSidebar(_props: CalendarSidebarProps) {
  return null;
}
