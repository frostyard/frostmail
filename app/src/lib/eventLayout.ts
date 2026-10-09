import type { Occurrence } from "../rpc/gen/api";
import { addDays, dayStart, zoned } from "./calendarDates";

/** TimedBlock is one day's part of a timed occurrence. */
export interface TimedBlock {
  occurrence: Occurrence;
  date: string;
  startMinute: number;
  endMinute: number;
  column: number;
  columns: number;
  continuesBefore: boolean;
  continuesAfter: boolean;
}

/** AllDayBar spans the inclusive first through last day indexes in a lane. */
export interface AllDayBar {
  occurrence: Occurrence;
  lane: number;
  first: number;
  last: number;
  continuesBefore: boolean;
  continuesAfter: boolean;
}

function overlaps(occurrence: Occurrence, from: number, to: number): boolean {
  const start = new Date(occurrence.start).getTime();
  const end = new Date(occurrence.end).getTime();
  return start < to && (start === end ? start >= from : end > from);
}

function dayBlocks(occurrences: Occurrence[], date: string, timeZone: string): TimedBlock[] {
  const from = dayStart(date, timeZone).getTime();
  const to = dayStart(addDays(date, 1), timeZone).getTime();
  return occurrences
    .filter((o) => !o.allDay && o.answer !== "declined" && overlaps(o, from, to))
    .map((occurrence) => {
      const start = new Date(occurrence.start).getTime();
      const end = new Date(occurrence.end).getTime();
      return {
        occurrence,
        date,
        startMinute: start < from ? 0 : zoned(occurrence.start, timeZone).minutes,
        endMinute: end >= to ? 1440 : zoned(occurrence.end, timeZone).minutes,
        continuesBefore: start < from,
        continuesAfter: end > to,
        column: 0,
        columns: 1,
      };
    })
    .sort((a, b) => a.startMinute - b.startMinute || b.endMinute - a.endMinute);
}

function assignColumns(blocks: TimedBlock[]): TimedBlock[] {
  let cluster: TimedBlock[] = [];
  let ends: number[] = [];
  let latest = -Infinity;
  const finish = () => {
    for (const block of cluster) block.columns = ends.length;
  };
  for (const block of blocks) {
    if (block.startMinute >= latest) {
      finish();
      cluster = [];
      ends = [];
    }
    const available = ends.findIndex((end) => end <= block.startMinute);
    block.column = available < 0 ? ends.length : available;
    const roomEnd = Math.max(block.endMinute, block.startMinute + 22.5);
    ends[block.column] = roomEnd;
    latest = Math.max(latest, roomEnd);
    cluster.push(block);
  }
  finish();
  return blocks.sort((a, b) => a.startMinute - b.startMinute || a.column - b.column);
}

/** timedLayout splits timed occurrences by day and assigns columns per cluster. */
export function timedLayout(occurrences: Occurrence[], days: string[], timeZone: string): TimedBlock[] {
  return days.flatMap((date) => assignColumns(dayBlocks(occurrences, date, timeZone)));
}

/** allDayLanes assigns each all-day bar the lowest available lane. */
export function allDayLanes(occurrences: Occurrence[], days: string[]): AllDayBar[] {
  const from = days[0];
  const lastDay = days.at(-1);
  if (from === undefined || lastDay === undefined) return [];
  const to = addDays(lastDay, 1);
  const bars = occurrences
    .filter((o) => o.allDay && o.answer !== "declined" && o.startDate < to && o.endDate > from)
    .map((occurrence) => ({
      occurrence,
      lane: 0,
      first: days.findIndex((date) => date >= occurrence.startDate),
      last: days.findLastIndex((date) => date < occurrence.endDate),
      continuesBefore: occurrence.startDate < from,
      continuesAfter: occurrence.endDate > to,
    }))
    .sort((a, b) => a.first - b.first || b.last - a.last);
  const ends: number[] = [];
  for (const bar of bars) {
    const available = ends.findIndex((end) => end < bar.first);
    bar.lane = available < 0 ? ends.length : available;
    ends[bar.lane] = bar.last;
  }
  return bars;
}

/** monthItems lists all-day occurrences first, then timed ones in start order. */
export function monthItems(occurrences: Occurrence[], date: string, timeZone: string): Occurrence[] {
  const from = dayStart(date, timeZone).getTime();
  const to = dayStart(addDays(date, 1), timeZone).getTime();
  return occurrences
    .filter(
      (o) => o.answer !== "declined" && (o.allDay ? o.startDate <= date && date < o.endDate : overlaps(o, from, to)),
    )
    .sort((a, b) => {
      if (a.allDay !== b.allDay) return a.allDay ? -1 : 1;
      if (a.allDay) return compareDates(a.startDate, b.startDate) || compareDates(b.endDate, a.endDate);
      return new Date(a.start).getTime() - new Date(b.start).getTime();
    });
}

function compareDates(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0;
}

/** busyDates returns the supplied days that have visible occurrences. */
export function busyDates(occurrences: Occurrence[], days: string[], timeZone: string): Set<string> {
  return new Set(days.filter((date) => monthItems(occurrences, date, timeZone).length > 0));
}
