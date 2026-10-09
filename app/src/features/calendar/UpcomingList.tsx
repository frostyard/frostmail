// A person's coming occurrences on the contact card and the person pane
// (docs/specs/pim-ui.md, Contact card). Task T-0074 builds it.
import type { Occurrence } from "../../rpc/gen/api";

/** UpcomingListProps are the Upcoming section's inputs. */
export interface UpcomingListProps {
  occurrences: Occurrence[];
  /** Each calendar's color: #rrggbb, or "" for the accent color. */
  colors: ReadonlyMap<number, string>;
  timeZone: string;
  locale: string;
  now: Date;
  onOpen: (occurrence: Occurrence) => void;
}

/** UpcomingList shows occurrences under an Upcoming heading, or nothing. */
export function UpcomingList(_props: UpcomingListProps) {
  return null;
}
