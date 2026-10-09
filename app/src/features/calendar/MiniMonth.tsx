// The Calendar sidebar's small month (docs/specs/pim-ui.md, Calendar
// sidebar). Task T-0071 builds it.

/** MiniMonthProps are the small month's inputs. */
export interface MiniMonthProps {
  /** The selected date, YYYY-MM-DD. */
  selected: string;
  today: string;
  /** The week's first day; 0 is Sunday. */
  weekStart: number;
  /** Dates with occurrences: a dot under the number. */
  busy: ReadonlySet<string>;
  locale: string;
  onSelect: (date: string) => void;
}

/** MiniMonth shows six weeks around the selected date's month. */
export function MiniMonth(_props: MiniMonthProps) {
  return null;
}
