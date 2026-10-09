import type { CSSProperties } from "react";
import { calendarColor } from "../../lib/eventText";
import type { Occurrence } from "../../rpc/gen/api";
import type { OccurrenceKey } from "./TimeGrid";

/** eventSelected compares the identity of a series occurrence. */
export function eventSelected(event: Occurrence, selected: OccurrenceKey | null): boolean {
  return event.eventId === selected?.eventId && event.recurrenceId === selected.recurrenceId;
}

/** eventStyle supplies the collection color for an occurrence. */
export function eventStyle(event: Occurrence, colors: ReadonlyMap<number, string>): CSSProperties {
  return { "--event-color": calendarColor(colors.get(event.calendarId) ?? "") } as CSSProperties;
}

/** eventClasses shares selection, cancellation and invitation appearance. */
export function eventClasses(event: Occurrence, selected: boolean, filled = true): string {
  const invitation = event.answer === "needsaction" || event.answer === "tentative";
  const fill = selected
    ? "bg-[var(--event-color)] text-accent-contrast"
    : invitation
      ? "bg-window"
      : filled
        ? "bg-[color-mix(in_srgb,var(--event-color)_18%,var(--bg-window))]"
        : "";
  const border = invitation
    ? "border border-dashed border-[var(--event-color)]"
    : filled
      ? "border-l-[3px] border-l-[var(--event-color)]"
      : "";
  return `${fill} ${border} ${event.status === "cancelled" ? "opacity-60" : ""}`;
}

/** eventTitleClasses strikes cancelled titles. */
export function eventTitleClasses(event: Occurrence): string {
  return event.status === "cancelled" ? "line-through" : "";
}
