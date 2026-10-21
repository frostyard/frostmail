import { Sheet } from "@frostyard/lodge-ui/react";
import { type KeyboardEvent, useEffect, useId, useRef, useState } from "react";
import { localeWeekStart } from "../../lib/calendarDates";
import { defaultRepeat, type Frequency, nthWeekday, type Repeat, WEEKDAYS, type Weekday } from "../../lib/repeat";
import { BUTTON, FIELD, PRIMARY_BUTTON } from "../settings/labels";

/** RepeatSheetProps supplies the starting rule and the sheet's callbacks. */
export interface RepeatSheetProps {
  initial: Repeat;
  start: string;
  locale: string;
  onChoose: (repeat: Repeat) => void;
  onCancel: () => void;
}

const DAYS: Record<Weekday, string> = {
  MO: "Monday",
  TU: "Tuesday",
  WE: "Wednesday",
  TH: "Thursday",
  FR: "Friday",
  SA: "Saturday",
  SU: "Sunday",
};
const FREQUENCIES: [Frequency, string, string][] = [
  ["DAILY", "Daily", "day"],
  ["WEEKLY", "Weekly", "week"],
  ["MONTHLY", "Monthly", "month"],
  ["YEARLY", "Yearly", "year"],
];
const ORDINALS: Record<string, string> = { "1": "first", "2": "second", "3": "third", "4": "fourth", "-1": "last" };

function sheetKey(event: KeyboardEvent<HTMLDivElement>, onCancel: () => void) {
  event.stopPropagation();
  if (event.key === "Escape") {
    event.preventDefault();
    onCancel();
  } else if (event.key === "Tab") {
    const controls = Array.from(event.currentTarget.querySelectorAll<HTMLElement>("input, select, button")).filter(
      (control) => !control.hasAttribute("disabled"),
    );
    const first = controls[0];
    const last = controls.at(-1);
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last?.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first?.focus();
    }
  }
}

function WeeklyDays({ repeat, locale, onChange }: { repeat: Repeat; locale: string; onChange: (r: Repeat) => void }) {
  const offset = (localeWeekStart(locale) + 6) % 7;
  const days = [...WEEKDAYS.slice(offset), ...WEEKDAYS.slice(0, offset)];
  return (
    <fieldset className="space-y-2">
      <legend className="text-[12px] leading-4 text-secondary">Days</legend>
      <div className="flex gap-1">
        {days.map((day) => (
          <button
            key={day}
            type="button"
            aria-label={DAYS[day]}
            aria-pressed={repeat.weekdays.includes(day)}
            className={`${BUTTON} flex-1 px-1 aria-pressed:bg-accent aria-pressed:text-accent-contrast`}
            onClick={() =>
              onChange({
                ...repeat,
                weekdays: repeat.weekdays.includes(day)
                  ? repeat.weekdays.filter((value) => value !== day)
                  : [...repeat.weekdays, day],
              })
            }
          >
            {DAYS[day].slice(0, 3)}
          </button>
        ))}
      </div>
    </fieldset>
  );
}

function MonthlyDays({ repeat, start, onChange }: { repeat: Repeat; start: string; onChange: (r: Repeat) => void }) {
  const name = useId();
  const nth = nthWeekday(start);
  const ordinal = ORDINALS[nth.slice(0, -2)];
  const day = DAYS[nth.slice(-2) as Weekday];
  return (
    <div className="space-y-2">
      {[false, true].map((byWeekday) => (
        <label key={String(byWeekday)} className="flex items-center gap-2 text-[13px] leading-[18px]">
          <input
            type="radio"
            name={name}
            checked={repeat.byWeekday === byWeekday}
            onChange={() => onChange({ ...repeat, byWeekday })}
          />
          {byWeekday ? `On the ${ordinal} ${day}` : `On day ${Number(start.slice(8, 10))}`}
        </label>
      ))}
    </div>
  );
}

/** RepeatSheet edits a recurrence without changing the event until OK. */
export function RepeatSheet({ initial, start, locale, onChoose, onCancel }: RepeatSheetProps) {
  const [repeat, setRepeat] = useState(initial);
  const [interval, setInterval] = useState(String(initial.interval));
  const amount = Number(interval);
  const valid =
    Number.isInteger(amount) && amount >= 1 && (repeat.frequency !== "WEEKLY" || repeat.weekdays.length > 0);
  const unit = FREQUENCIES.find(([frequency]) => frequency === repeat.frequency)?.[2];
  return (
    <Sheet
      title="Custom Repeat"
      width={420}
      className="rounded-[10px]"
      titleClassName="text-[13px] font-semibold"
      showClose={false}
      closeOnOverlay={false}
      initialFocus={"first"}
      onClose={onCancel}
    >
      {/* biome-ignore lint/a11y/noStaticElementInteractions: the Sheet's frame is the dialog; this wrapper only keeps Tab and the keys inside it */}
      <div
        onKeyDown={(event) => {
          if (event.key === "Tab") sheetKey(event, onCancel);
        }}
        onKeyUp={(event) => event.stopPropagation()}
      >
        <form
          className="space-y-3"
          onSubmit={(event) => {
            event.preventDefault();
            event.stopPropagation();
            if (valid)
              onChoose({
                ...repeat,
                interval: amount,
                weekdays: WEEKDAYS.filter((day) => repeat.weekdays.includes(day)),
              });
          }}
        >
          <label className="flex items-center gap-2 text-[13px]">
            Frequency
            <select
              className={`${FIELD} flex-1`}
              value={repeat.frequency}
              onChange={(event) => {
                const frequency = event.currentTarget.value as Frequency;
                setRepeat({
                  ...repeat,
                  frequency,
                  weekdays:
                    frequency === "WEEKLY" && repeat.weekdays.length === 0
                      ? defaultRepeat(start).weekdays
                      : repeat.weekdays,
                });
              }}
            >
              {FREQUENCIES.map(([frequency, label]) => (
                <option key={frequency} value={frequency}>
                  {label}
                </option>
              ))}
            </select>
          </label>
          <div className="flex items-center gap-2 text-[13px]">
            <label className="flex items-center gap-2">
              Every
              <input
                type="number"
                min={1}
                step={1}
                className={`${FIELD} w-20`}
                value={interval}
                onChange={(event) => setInterval(event.currentTarget.value)}
              />
            </label>
            <span>
              {unit}
              {amount === 1 ? "" : "s"}
            </span>
          </div>
          {repeat.frequency === "WEEKLY" && <WeeklyDays repeat={repeat} locale={locale} onChange={setRepeat} />}
          {repeat.frequency === "MONTHLY" && <MonthlyDays repeat={repeat} start={start} onChange={setRepeat} />}
          <div className="flex justify-end gap-2">
            <button type="button" className={BUTTON} onClick={onCancel}>
              Cancel
            </button>
            <button type="submit" className={PRIMARY_BUTTON} disabled={!valid}>
              OK
            </button>
          </div>
        </form>
      </div>
    </Sheet>
  );
}
