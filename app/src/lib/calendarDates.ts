/** CalendarView is the period shown by a calendar view. */
export type CalendarView = "day" | "week" | "month";

/** addDays moves a date by whole UTC days. */
export function addDays(date: string, n: number): string {
  const value = new Date(date);
  value.setUTCDate(value.getUTCDate() + n);
  return value.toISOString().slice(0, 10);
}

/** weekday returns the date's weekday, with Sunday as 0. */
export function weekday(date: string): number {
  return new Date(date).getUTCDay();
}

/** startOfWeek returns the first day of the week containing date. */
export function startOfWeek(date: string, weekStart: number): string {
  return addDays(date, -((weekday(date) - weekStart + 7) % 7));
}

/** monthGrid returns six weeks starting with the week containing the 1st. */
export function monthGrid(date: string, weekStart: number): string[] {
  const from = startOfWeek(`${date.slice(0, 7)}-01`, weekStart);
  return Array.from({ length: 42 }, (_, index) => addDays(from, index));
}

/** viewRange returns the view's inclusive start and exclusive end. */
export function viewRange(view: CalendarView, date: string, weekStart: number): { from: string; to: string } {
  const from = view === "day" ? date : startOfWeek(view === "month" ? `${date.slice(0, 7)}-01` : date, weekStart);
  return { from, to: addDays(from, view === "day" ? 1 : view === "week" ? 7 : 42) };
}

/** step moves by a view period, clamping the day when changing months. */
export function step(view: CalendarView, date: string, direction: 1 | -1): string {
  if (view !== "month") return addDays(date, direction * (view === "day" ? 1 : 7));
  const value = new Date(date);
  const day = value.getUTCDate();
  value.setUTCDate(1);
  value.setUTCMonth(value.getUTCMonth() + direction);
  const last = new Date(value);
  last.setUTCMonth(last.getUTCMonth() + 1);
  last.setUTCDate(0);
  value.setUTCDate(Math.min(day, last.getUTCDate()));
  return value.toISOString().slice(0, 10);
}

/** viewTitle formats a day, month, or the months spanned by a week. */
export function viewTitle(view: CalendarView, date: string, weekStart: number, locale: string): string {
  const format = (day: string, options: Intl.DateTimeFormatOptions) =>
    new Intl.DateTimeFormat(locale, { ...options, timeZone: "UTC" }).format(new Date(day));
  if (view === "day") return format(date, { weekday: "long", month: "long", day: "numeric", year: "numeric" });
  if (view === "month") return format(date, { month: "long", year: "numeric" });
  const first = startOfWeek(date, weekStart);
  const last = addDays(first, 6);
  if (first.slice(0, 7) === last.slice(0, 7)) return format(first, { month: "long", year: "numeric" });
  const firstOptions: Intl.DateTimeFormatOptions = { month: "short" };
  if (first.slice(0, 4) !== last.slice(0, 4)) firstOptions.year = "numeric";
  return `${format(first, firstOptions)} – ${format(last, { month: "short", year: "numeric" })}`;
}

/** zoned reads an instant's date and wall-clock minutes in a time zone. */
export function zoned(instant: string | Date, timeZone: string): { date: string; minutes: number } {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(typeof instant === "string" ? new Date(instant) : instant);
  const part = (type: Intl.DateTimeFormatPartTypes) => parts.find((p) => p.type === type)?.value ?? "";
  return {
    date: `${part("year")}-${part("month")}-${part("day")}`,
    minutes: Number(part("hour")) * 60 + Number(part("minute")),
  };
}

/** today names the current date in the supplied time zone. */
export function today(timeZone: string, now: Date): string {
  return zoned(now, timeZone).date;
}

/** dayStart finds the instant at the date's midnight in the supplied zone. */
export function dayStart(date: string, timeZone: string): Date {
  const target = new Date(date).getTime();
  let instant = target;
  for (let correction = 0; correction < 3; correction++) {
    const wall = zoned(new Date(instant), timeZone);
    const difference = target - (new Date(wall.date).getTime() + wall.minutes * 60_000);
    instant += difference;
    if (difference === 0) break;
  }
  return new Date(instant);
}

/** localeWeekStart reads locale week information, falling back to Sunday. */
export function localeWeekStart(locale: string): number {
  try {
    const value = new Intl.Locale(locale) as Intl.Locale & {
      getWeekInfo?: () => { firstDay: number };
      weekInfo?: { firstDay: number };
    };
    const first = (value.getWeekInfo?.() ?? value.weekInfo)?.firstDay;
    return first !== undefined && first >= 1 && first <= 7 ? first % 7 : 0;
  } catch {
    return 0;
  }
}
