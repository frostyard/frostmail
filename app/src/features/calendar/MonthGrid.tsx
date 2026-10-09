// The month view (docs/specs/pim-ui.md, Month view). Task T-0071 builds it.
import type { Occurrence } from "../../rpc/gen/api";
import type { OccurrenceKey } from "./TimeGrid";

/** MonthGridProps are the month view's inputs. */
export interface MonthGridProps {
  /** The 42 days shown (monthGrid). */
  days: string[];
  /** The selected date; its month is the one shown, the others dimmed. */
  selectedDate: string;
  occurrences: Occurrence[];
  colors: ReadonlyMap<number, string>;
  timeZone: string;
  locale: string;
  today: string;
  selected: OccurrenceKey | null;
  /** The most lines a cell shows, "N more" included; at least 2. */
  lines: number;
  onSelect: (occurrence: Occurrence) => void;
  onSelectDate: (date: string) => void;
  onShowDay: (date: string) => void;
}

/** MonthGrid shows six weeks of days and their occurrences. */
export function MonthGrid(_props: MonthGridProps) {
  return null;
}
