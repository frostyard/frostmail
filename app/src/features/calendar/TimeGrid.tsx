// Fixed day headings and all-day lanes over a scrolling time grid.
import { type PointerEvent as ReactPointerEvent, useEffect, useRef, useState } from "react";
import { viewTitle, zoned } from "../../lib/calendarDates";
import {
  daysOf,
  drawn,
  minuteAt,
  moved,
  PX_PER_MINUTE,
  type Range,
  resized,
  stepsOf,
  THRESHOLD,
} from "../../lib/eventDrag";
import { allDayLanes, type TimedBlock, timedLayout } from "../../lib/eventLayout";
import { timeRange } from "../../lib/eventText";
import type { Occurrence } from "../../rpc/gen/api";
import { eventClasses, eventSelected, eventStyle, eventTitleClasses } from "./eventStyle";

/** OccurrenceKey names one occurrence. */
export interface OccurrenceKey {
  eventId: number;
  recurrenceId: string;
}

/** TimeGridProps are the day and week views' inputs. */
export interface TimeGridProps {
  /** The days shown: one (the day view) or seven (the week view). */
  days: string[];
  occurrences: Occurrence[];
  /** Each calendar's color: #rrggbb, or "" for the accent color. */
  colors: ReadonlyMap<number, string>;
  timeZone: string;
  locale: string;
  today: string;
  /** The current time, for the now line. */
  now: Date;
  selected: OccurrenceKey | null;
  onSelect: (occurrence: Occurrence) => void;
  /** Show a day in the day view. */
  onShowDay: (date: string) => void;
  canMove?: (occurrence: Occurrence) => boolean;
  onMove?: (occurrence: Occurrence, range: Range) => void;
  onDraw?: (range: Range) => void;
}

interface Press {
  pointerId: number;
  x: number;
  y: number;
  day: number;
  date: string;
  rect: DOMRect;
  action: { kind: "draw" } | { kind: "move" | "resize"; occurrence: Occurrence };
  dragging: boolean;
}

interface Preview {
  range: Range;
  occurrence?: Occurrence;
}

function pressRange(press: Press, x: number, y: number, props: TimeGridProps): Range {
  const { action, rect } = press;
  if (action.kind === "draw") {
    return drawn(press.date, minuteAt(press.y - rect.top), minuteAt(y - rect.top), props.timeZone);
  }
  const minutes = stepsOf(y - press.y);
  if (action.kind === "resize") return resized(action.occurrence, minutes, props.timeZone);
  const days = Math.max(-press.day, Math.min(props.days.length - 1 - press.day, daysOf(x - press.x, rect.width)));
  return moved(action.occurrence, days, minutes, props.timeZone);
}

function sameRange(a: Range, b: Range): boolean {
  return Date.parse(a.start) === Date.parse(b.start) && Date.parse(a.end) === Date.parse(b.end);
}

function useTimeGridDrag(props: TimeGridProps) {
  const press = useRef<Press | null>(null);
  const skipClick = useRef(false);
  const clickTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const [preview, setPreview] = useState<Preview | null>(null);
  useEffect(() => {
    const follow = (event: PointerEvent) => {
      const current = press.current;
      if (!current || current.pointerId !== event.pointerId) return null;
      current.dragging ||=
        Math.max(Math.abs(event.clientX - current.x), Math.abs(event.clientY - current.y)) >= THRESHOLD;
      if (!current.dragging) return null;
      return pressRange(current, event.clientX, event.clientY, props);
    };
    const move = (event: PointerEvent) => {
      const range = follow(event);
      const action = press.current?.action;
      if (range && action) setPreview({ range, occurrence: action.kind === "draw" ? undefined : action.occurrence });
    };
    const end = () => {
      press.current = null;
      setPreview(null);
    };
    const up = (event: PointerEvent) => {
      const current = press.current;
      if (!current || current.pointerId !== event.pointerId) return;
      const range = follow(event);
      end();
      if (!range || (current.action.kind !== "draw" && sameRange(range, current.action.occurrence))) return;
      skipClick.current = true;
      clearTimeout(clickTimer.current);
      clickTimer.current = setTimeout(() => {
        skipClick.current = false;
      }, 0);
      if (current.action.kind === "draw") props.onDraw?.(range);
      else props.onMove?.(current.action.occurrence, range);
    };
    const cancel = (event: PointerEvent) => {
      if (press.current?.pointerId === event.pointerId) end();
    };
    const onEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || !press.current) return;
      event.preventDefault();
      event.stopImmediatePropagation();
      end();
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("pointercancel", cancel);
    window.addEventListener("keydown", onEscape, true);
    return () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("pointercancel", cancel);
      window.removeEventListener("keydown", onEscape, true);
    };
  }, [props]);
  useEffect(() => () => clearTimeout(clickTimer.current), []);

  const begin = (event: ReactPointerEvent<HTMLElement>, day: number, occurrence?: Occurrence) => {
    if (event.button !== 0 || press.current) return;
    if (occurrence ? !props.canMove?.(occurrence) : !props.onDraw) return;
    const column = event.currentTarget.closest("fieldset");
    const date = props.days[day];
    if (!column || !date) return;
    skipClick.current = false;
    const resize = event.target instanceof Element && event.target.closest("[data-resize]") !== null;
    press.current = {
      pointerId: event.pointerId,
      x: event.clientX,
      y: event.clientY,
      day,
      date,
      rect: column.getBoundingClientRect(),
      dragging: false,
      action: occurrence ? { kind: resize ? "resize" : "move", occurrence } : { kind: "draw" },
    };
  };
  const select = (occurrence: Occurrence) => {
    if (!skipClick.current) props.onSelect(occurrence);
    skipClick.current = false;
  };
  return { begin, select, preview };
}

function DragPreview({ preview, props }: { preview: Preview; props: TimeGridProps }) {
  const { range, occurrence } = preview;
  const start = zoned(range.start, props.timeZone);
  const minutes = (Date.parse(range.end) - Date.parse(range.start)) / 60_000;
  return (
    <div
      data-drag-preview=""
      className={`pointer-events-none absolute inset-x-0 z-20 rounded-[4px] p-1 opacity-60 ${occurrence ? eventClasses(occurrence, false) : "bg-accent text-accent-contrast"}`}
      style={{
        ...(occurrence ? eventStyle(occurrence, props.colors) : {}),
        top: `${start.minutes * PX_PER_MINUTE}px`,
        height: `${Math.max(minutes * PX_PER_MINUTE, 18)}px`,
      }}
    >
      <div className="truncate text-[12px] font-semibold leading-[15px]">
        {occurrence ? occurrence.summary || "No Title" : "New Event"}
      </div>
      <div className="truncate text-[11px] leading-[14px]">
        {timeRange(range.start, range.end, props.timeZone, props.locale)}
      </div>
    </div>
  );
}

function AllDayStrip({ props }: { props: TimeGridProps }) {
  const bars = allDayLanes(props.occurrences, props.days);
  const lanes = Math.max(0, ...bars.map((bar) => bar.lane + 1));
  const collapse = props.days.length === 7 && lanes > 3;
  return (
    <div className="flex border-b border-separator">
      <span className="w-14 shrink-0 pr-2 text-right text-[11px] leading-[14px] text-tertiary">
        {lanes > 0 && "all-day"}
      </span>
      <div
        className="grid flex-1 gap-y-[2px] py-1"
        style={{
          gridTemplateColumns: `repeat(${props.days.length}, minmax(0, 1fr))`,
          gridTemplateRows: `repeat(${collapse ? 3 : lanes}, 22px)`,
        }}
      >
        {bars
          .filter((bar) => !collapse || bar.lane < 2)
          .map((bar) => (
            <button
              key={`${bar.occurrence.eventId}:${bar.occurrence.recurrenceId}`}
              type="button"
              aria-label={`${bar.occurrence.summary || "No Title"}, all day`}
              aria-pressed={eventSelected(bar.occurrence, props.selected)}
              className={`mx-[2px] truncate rounded-[4px] px-1 text-left text-[12px] font-semibold ${eventClasses(bar.occurrence, eventSelected(bar.occurrence, props.selected))}`}
              style={{
                ...eventStyle(bar.occurrence, props.colors),
                gridColumn: `${bar.first + 1} / ${bar.last + 2}`,
                gridRow: bar.lane + 1,
              }}
              onClick={() => props.onSelect(bar.occurrence)}
            >
              <span className={eventTitleClasses(bar.occurrence)}>{bar.occurrence.summary || "No Title"}</span>
            </button>
          ))}
        {collapse &&
          props.days.map((date, index) => {
            const count = bars.filter((bar) => bar.lane >= 2 && bar.first <= index && bar.last >= index).length;
            return (
              count > 0 && (
                <button
                  key={date}
                  type="button"
                  className="text-left text-[11px] text-secondary"
                  style={{ gridColumn: index + 1, gridRow: 3 }}
                  aria-label={`Show ${count} more on ${viewTitle("day", date, 0, props.locale)}`}
                  onClick={() => props.onShowDay(date)}
                >
                  {count} more
                </button>
              )
            );
          })}
      </div>
    </div>
  );
}

function EventBlock({
  block,
  props,
  onPress,
  onSelect,
}: {
  block: TimedBlock;
  props: TimeGridProps;
  onPress: (event: ReactPointerEvent<HTMLButtonElement>) => void;
  onSelect: (occurrence: Occurrence) => void;
}) {
  const occurrence = block.occurrence;
  const height = Math.max((block.endMinute - block.startMinute) * 0.8, 18);
  const range = timeRange(occurrence.start, occurrence.end, props.timeZone, props.locale);
  const active = eventSelected(occurrence, props.selected);
  return (
    <button
      type="button"
      aria-label={`${occurrence.summary || "No Title"}, ${range}${occurrence.location ? `, ${occurrence.location}` : ""}`}
      aria-pressed={active}
      onClick={() => onSelect(occurrence)}
      onPointerDown={onPress}
      className={`absolute overflow-hidden rounded-[4px] p-1 text-left ${props.canMove?.(occurrence) ? "touch-none select-none" : ""} ${eventClasses(occurrence, active)}`}
      style={{
        ...eventStyle(occurrence, props.colors),
        top: `${block.startMinute * 0.8}px`,
        height: `${height}px`,
        left: `${(block.column / block.columns) * 100}%`,
        width: `calc(${100 / block.columns}% - 4px)`,
      }}
    >
      <div className={`truncate text-[12px] font-semibold leading-[15px] ${eventTitleClasses(occurrence)}`}>
        {occurrence.summary || "No Title"}
      </div>
      {height >= 36 && (
        <div className={`text-[11px] leading-[14px] ${active ? "text-accent-contrast" : "text-secondary"}`}>
          <div className="truncate">{range}</div>
          {occurrence.location && <div className="truncate">{occurrence.location}</div>}
        </div>
      )}
      {props.canMove?.(occurrence) && (
        <span data-resize="" aria-hidden="true" className="absolute inset-x-0 bottom-0 h-[6px] cursor-ns-resize" />
      )}
    </button>
  );
}

/** TimeGrid shows the days' all-day strip and hours. */
export function TimeGrid(props: TimeGridProps) {
  const drag = useTimeGridDrag(props);
  const scroll = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (scroll.current) scroll.current.scrollTop = 7 * 48 - 12;
  }, []);
  const blocks = timedLayout(props.occurrences, props.days, props.timeZone);
  const now = zoned(props.now, props.timeZone);
  const time = new Intl.DateTimeFormat(props.locale, {
    hour: "numeric",
    minute: "2-digit",
    timeZone: props.timeZone,
  }).format(props.now);
  return (
    <div className="flex h-full min-h-0 flex-col bg-window">
      <header className="flex h-12 shrink-0 border-b border-separator">
        <div className="w-14 shrink-0" />
        {props.days.map((date) => (
          <button
            key={date}
            type="button"
            aria-label={viewTitle("day", date, 0, props.locale)}
            aria-current={date === props.today ? "date" : undefined}
            onClick={() => props.onShowDay(date)}
            className="flex min-w-0 flex-1 flex-col items-center justify-center"
          >
            <span className="text-[11px] leading-[14px] text-secondary">
              {new Intl.DateTimeFormat(props.locale, { weekday: "short", timeZone: "UTC" }).format(new Date(date))}
            </span>
            <span
              className={`flex size-6 items-center justify-center rounded-full text-[15px] leading-5 ${date === props.today ? "bg-accent text-accent-contrast" : ""}`}
            >
              {new Intl.DateTimeFormat(props.locale, { day: "numeric", timeZone: "UTC" }).format(new Date(date))}
            </span>
          </button>
        ))}
      </header>
      <AllDayStrip props={props} />
      <div ref={scroll} className="min-h-0 flex-1 overflow-y-auto">
        <div className="flex h-[1152px]">
          <div className="relative w-14 shrink-0">
            {Array.from({ length: 23 }, (_, index) => index + 1).map((hour) => (
              <span
                key={hour}
                className="absolute right-2 -translate-y-1/2 text-[11px] leading-[14px] text-tertiary"
                style={{ top: `${hour * 48}px` }}
              >
                {new Intl.DateTimeFormat(props.locale, { hour: "numeric", timeZone: "UTC" }).format(
                  new Date(Date.UTC(2000, 0, 1, hour)),
                )}
              </span>
            ))}
            {props.days.includes(props.today) && (
              <span
                className="absolute right-2 -translate-y-1/2 bg-window text-[11px] font-semibold leading-[14px] text-flag-1"
                style={{ top: `${now.minutes * 0.8}px` }}
              >
                {time}
              </span>
            )}
          </div>
          {props.days.map((date, day) => (
            <fieldset
              key={date}
              aria-label={viewTitle("day", date, 0, props.locale)}
              className={`relative min-w-0 flex-1 border-l border-separator ${props.onDraw ? "touch-none select-none" : ""}`}
              onPointerDown={(event) => {
                if (event.target === event.currentTarget) drag.begin(event, day);
              }}
            >
              {Array.from({ length: 24 }, (_, hour) => hour).map((hour) => (
                <div
                  key={hour}
                  className="pointer-events-none absolute w-full border-t border-separator"
                  style={{ top: `${hour * 48}px` }}
                />
              ))}
              {blocks
                .filter((block) => block.date === date)
                .map((block) => (
                  <EventBlock
                    key={`${block.occurrence.eventId}:${block.occurrence.recurrenceId}`}
                    block={block}
                    props={props}
                    onPress={(event) => drag.begin(event, day, block.occurrence)}
                    onSelect={drag.select}
                  />
                ))}
              {drag.preview && zoned(drag.preview.range.start, props.timeZone).date === date && (
                <DragPreview preview={drag.preview} props={props} />
              )}
              {date === props.today && (
                <div
                  data-now=""
                  className="pointer-events-none absolute z-10 h-[2px] w-full bg-flag-1"
                  style={{ top: `${now.minutes * 0.8}px` }}
                >
                  <span className="absolute -left-1 -top-[3px] size-2 rounded-full bg-flag-1" />
                </div>
              )}
            </fieldset>
          ))}
        </div>
      </div>
    </div>
  );
}
