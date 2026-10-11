import { useEffect, useId, useRef, useState } from "react";

import { zoned } from "../../lib/calendarDates";
import { atLocal } from "../../lib/later";
import { BUTTON, FIELD, PRIMARY_BUTTON } from "../settings/labels";

/** TimeSheetProps supplies the starting time, zone and choice callbacks. */
export interface TimeSheetProps {
  title: string;
  initial: Date;
  now: Date;
  timeZone: string;
  onChoose: (at: Date) => void;
  onCancel: () => void;
}

/** TimeSheet asks for a future date and wall-clock time in the supplied zone. */
export function TimeSheet(props: TimeSheetProps) {
  const [date, setDate] = useState(() => zoned(props.initial, props.timeZone).date);
  const [time, setTime] = useState(() => {
    const { minutes } = zoned(props.initial, props.timeZone);
    return `${String(Math.floor(minutes / 60)).padStart(2, "0")}:${String(minutes % 60).padStart(2, "0")}`;
  });
  const titleId = useId();
  const dialog = useRef<HTMLDivElement>(null);
  const at =
    date && time ? atLocal(date, Number(time.slice(0, 2)), Number(time.slice(3, 5)), props.timeZone) : undefined;
  const valid = at !== undefined && at > props.now;
  useEffect(() => {
    const previous = document.activeElement;
    dialog.current?.querySelector<HTMLInputElement>("input")?.focus();
    return () => {
      if (previous instanceof HTMLElement) previous.focus();
    };
  }, []);
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-window/60">
      <div
        ref={dialog}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        className="w-[360px] max-h-[90vh] overflow-y-auto rounded-[10px] border border-separator bg-window p-5 outline-none"
        onKeyDown={(event) => {
          event.stopPropagation();
          if (event.key === "Escape") {
            event.preventDefault();
            props.onCancel();
          } else if (event.key === "Tab") {
            const controls = Array.from(event.currentTarget.querySelectorAll<HTMLElement>("input, button")).filter(
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
        }}
      >
        <h2 id={titleId} className="mb-4 text-[13px] font-semibold">
          {props.title}
        </h2>
        <form
          className="space-y-3"
          onSubmit={(event) => {
            event.preventDefault();
            if (valid && at) props.onChoose(at);
          }}
        >
          <label className="flex items-center gap-2 text-[13px]">
            Date
            <input
              type="date"
              className={`${FIELD} min-w-0 flex-1`}
              value={date}
              onChange={(event) => setDate(event.currentTarget.value)}
            />
          </label>
          <label className="flex items-center gap-2 text-[13px]">
            Time
            <input
              type="time"
              className={`${FIELD} min-w-0 flex-1`}
              value={time}
              onChange={(event) => setTime(event.currentTarget.value)}
            />
          </label>
          <div className="flex justify-end gap-2">
            <button type="button" className={BUTTON} onClick={props.onCancel}>
              Cancel
            </button>
            <button type="submit" className={PRIMARY_BUTTON} disabled={!valid}>
              OK
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
