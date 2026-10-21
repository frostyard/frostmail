import { Sheet } from "@frostyard/lodge-ui/react";
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
  const at =
    date && time ? atLocal(date, Number(time.slice(0, 2)), Number(time.slice(3, 5)), props.timeZone) : undefined;
  const valid = at !== undefined && at > props.now;
  return (
    <Sheet
      title={props.title}
      width={360}
      className="rounded-[10px]"
      titleClassName="text-[13px] font-semibold"
      showClose={false}
      closeOnOverlay={false}
      initialFocus={"first"}
      onClose={props.onCancel}
    >
      {/* biome-ignore lint/a11y/noStaticElementInteractions: the Sheet's frame is the dialog; this wrapper only keeps Tab and the keys inside it */}
      <div
        onKeyDown={(event) => {
          if (event.key !== "Tab") return;
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
        }}
      >
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
    </Sheet>
  );
}
