import { Lock } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { type DraftErrors, type EventDraft, moveStart } from "../../lib/eventDraft";
import { alarmText, calendarColor, describeRecurrence } from "../../lib/eventText";
import {
  defaultRepeat,
  endOf,
  presetOf,
  presetRule,
  type RepeatEnd,
  type RepeatPreset,
  repeatOf,
  ruleOf,
  withEnd,
} from "../../lib/repeat";
import type { Address } from "../../rpc/gen/api";
import { FIELD } from "../settings/labels";
import { InviteesField } from "./InviteesField";
import { RepeatSheet } from "./RepeatSheet";

/** CalendarChoice names a writable calendar and its account. */
export interface CalendarChoice {
  id: number;
  name: string;
  color: string;
  account: string;
  invites?: boolean;
}

/** EventEditorProps supplies the controlled draft and available choices. */
export interface EventEditorProps {
  draft: EventDraft;
  onChange: (draft: EventDraft) => void;
  errors: DraftErrors;
  calendars: CalendarChoice[];
  zones: string[];
  locale: string;
  readOnly?: boolean;
  invited?: boolean;
  suggest?: (prefix: string) => Promise<Address[]>;
}

/** TIMED_ALERTS lists the timed event alert presets in menu order. */
export const TIMED_ALERTS: [minutes: number, label: string][] = [
  [0, "At time of event"],
  [5, "5 minutes before"],
  [10, "10 minutes before"],
  [15, "15 minutes before"],
  [30, "30 minutes before"],
  [60, "1 hour before"],
  [120, "2 hours before"],
  [1440, "1 day before"],
  [2880, "2 days before"],
  [10080, "1 week before"],
];

/** ALL_DAY_ALERTS lists the all-day event alert presets in menu order. */
export const ALL_DAY_ALERTS: [minutes: number, label: string][] = [
  [-540, "On the day at 9:00 AM"],
  [900, "1 day before at 9:00 AM"],
  [2340, "2 days before at 9:00 AM"],
  [9540, "1 week before at 9:00 AM"],
];

const PRESETS: [Exclude<RepeatPreset, "custom">, string][] = [
  ["never", "Never"],
  ["daily", "Every Day"],
  ["weekdays", "Every Weekday"],
  ["weekly", "Every Week"],
  ["biweekly", "Every 2 Weeks"],
  ["monthly", "Every Month"],
  ["yearly", "Every Year"],
];
const INPUT = `${FIELD} h-8 min-w-0 w-full disabled:opacity-40 aria-invalid:border-flag-1`;
const MESSAGE = "mt-1 text-[12px] leading-4 text-flag-1";

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid min-h-8 grid-cols-[80px_minmax(0,1fr)] items-start gap-x-3">
      <span className="pt-2 text-right text-[12px] leading-4 text-secondary">{label}</span>
      <div>{children}</div>
    </div>
  );
}

type DraftControls = Pick<EventEditorProps, "draft" | "onChange"> & { disabled?: boolean };

function DateRow({
  draft,
  onChange,
  disabled,
  part,
  error,
}: DraftControls & { part: "start" | "end"; error?: string }) {
  const id = useId();
  const label = part === "start" ? "Start" : "End";
  const invalid = { "aria-invalid": Boolean(error), "aria-describedby": error ? id : undefined };
  return (
    <Row label={part === "start" ? "Starts" : "Ends"}>
      <div className="flex gap-2">
        <input
          aria-label={`${label} date`}
          type="date"
          className={INPUT}
          disabled={disabled}
          {...invalid}
          value={draft[`${part}Date`]}
          onChange={(event) =>
            onChange(
              part === "start"
                ? moveStart(draft, event.currentTarget.value)
                : { ...draft, endDate: event.currentTarget.value },
            )
          }
        />
        {!draft.allDay && (
          <input
            aria-label={`${label} time`}
            type="time"
            className={INPUT}
            disabled={disabled}
            {...invalid}
            value={draft[`${part}Time`]}
            onChange={(event) =>
              onChange(
                part === "start"
                  ? moveStart(draft, draft.startDate, event.currentTarget.value)
                  : { ...draft, endTime: event.currentTarget.value },
              )
            }
          />
        )}
      </div>
      {error && (
        <p id={id} className={MESSAGE}>
          {error}
        </p>
      )}
    </Row>
  );
}

function EndRepeat({ draft, onChange, disabled }: DraftControls) {
  const end = endOf(draft.rrule, draft.timeZone);
  const change = (value: RepeatEnd) =>
    onChange({ ...draft, rrule: withEnd(draft.rrule, value, draft.allDay, draft.timeZone) });
  return (
    <Row label="End repeat">
      <div className="flex items-center gap-2 text-[13px]">
        <select
          aria-label="End repeat"
          className={INPUT}
          disabled={disabled}
          value={end.kind}
          onChange={(event) =>
            change(
              event.currentTarget.value === "date"
                ? { kind: "date", date: draft.startDate }
                : event.currentTarget.value === "count"
                  ? { kind: "count", count: 10 }
                  : { kind: "never" },
            )
          }
        >
          <option value="never">Never</option>
          <option value="date">On Date</option>
          <option value="count">After</option>
        </select>
        {end.kind === "date" && (
          <input
            aria-label="End repeat date"
            type="date"
            className={INPUT}
            disabled={disabled}
            value={end.date}
            onChange={(event) => {
              if (event.currentTarget.value) change({ kind: "date", date: event.currentTarget.value });
            }}
          />
        )}
        {end.kind === "count" && (
          <>
            <input
              aria-label="Repeat count"
              type="number"
              min={1}
              step={1}
              className={`${INPUT} max-w-20`}
              disabled={disabled}
              value={end.count}
              onChange={(event) => change({ kind: "count", count: Number(event.currentTarget.value) })}
            />
            <span>times</span>
          </>
        )}
      </div>
    </Row>
  );
}

function Alerts({ draft, onChange, disabled }: DraftControls) {
  const presets = draft.allDay ? ALL_DAY_ALERTS : TIMED_ALERTS;
  const next = presets.find(([minutes]) => !draft.alarms.includes(minutes));
  return (
    <Row label="Alerts">
      <div className="space-y-1">
        {draft.alarms.map((alarm, index) => (
          <select
            // biome-ignore lint/suspicious/noArrayIndexKey: An alert's list position is its only identity.
            key={index}
            aria-label={`Alert ${index + 1}`}
            className={INPUT}
            disabled={disabled}
            value={alarm}
            onChange={(event) =>
              onChange({
                ...draft,
                alarms:
                  event.currentTarget.value === "none"
                    ? draft.alarms.filter((_, position) => position !== index)
                    : draft.alarms.map((value, position) =>
                        position === index ? Number(event.currentTarget.value) : value,
                      ),
              })
            }
          >
            <option value="none">None</option>
            {presets.map(([minutes, label]) => (
              <option key={minutes} value={minutes}>
                {label}
              </option>
            ))}
            {!presets.some(([minutes]) => minutes === alarm) && <option value={alarm}>{alarmText(alarm)}</option>}
          </select>
        ))}
        <button
          type="button"
          className="text-[12px] leading-4 text-accent hover:underline disabled:opacity-40"
          disabled={disabled || !next}
          onClick={() => {
            if (next) onChange({ ...draft, alarms: [...draft.alarms, next[0]] });
          }}
        >
          Add Alert
        </button>
      </div>
    </Row>
  );
}

function CalendarSelect({ draft, onChange, disabled, calendars }: DraftControls & { calendars: CalendarChoice[] }) {
  return (
    <Row label="Calendar">
      <select
        aria-label="Calendar"
        className={INPUT}
        disabled={disabled}
        value={draft.calendarId}
        onChange={(event) => onChange({ ...draft, calendarId: Number(event.currentTarget.value) })}
      >
        {[...new Set(calendars.map((calendar) => calendar.account))].map((account) => (
          <optgroup key={account} label={account}>
            {calendars
              .filter((calendar) => calendar.account === account)
              .map((calendar) => (
                <option key={calendar.id} value={calendar.id} style={{ color: calendarColor(calendar.color) }}>
                  ● {calendar.name}
                </option>
              ))}
          </optgroup>
        ))}
      </select>
    </Row>
  );
}

/** EventEditor renders a controlled event draft and its Custom Repeat sheet. */
export function EventEditor({
  draft,
  onChange,
  errors,
  calendars,
  zones,
  locale,
  readOnly,
  invited,
  suggest = async () => [],
}: EventEditorProps) {
  const [repeatOpen, setRepeatOpen] = useState(false);
  const zoneErrorId = useId();
  const disabled = Boolean(readOnly || invited);
  const controls = { draft, onChange, disabled };
  const preset = presetOf(draft.rrule, draft.startDate);
  const availableZones = zones.includes(draft.timeZone) ? zones : [...zones, draft.timeZone];
  const changeRule = (rule: string) =>
    onChange({ ...draft, rrule: withEnd(rule, endOf(draft.rrule, draft.timeZone), draft.allDay, draft.timeZone) });
  return (
    <div className="space-y-2 p-5">
      <input
        aria-label="Title"
        placeholder="New Event"
        className={`${INPUT} text-[17px] leading-[22px] font-semibold`}
        disabled={disabled}
        value={draft.summary}
        onChange={(event) => onChange({ ...draft, summary: event.currentTarget.value })}
      />
      {readOnly && (
        <p className="flex items-center gap-1 text-[12px] leading-4 text-secondary">
          <Lock size={12} aria-hidden="true" />
          Read-only
        </p>
      )}
      <input
        aria-label="Location"
        placeholder="Add Location"
        className={INPUT}
        disabled={disabled}
        value={draft.location}
        onChange={(event) => onChange({ ...draft, location: event.currentTarget.value })}
      />
      <Row label="All-day">
        <input
          aria-label="All-day"
          type="checkbox"
          className="my-2"
          disabled={disabled}
          checked={draft.allDay}
          onChange={(event) => onChange({ ...draft, allDay: event.currentTarget.checked })}
        />
      </Row>
      <DateRow {...controls} part="start" error={errors.start} />
      <DateRow {...controls} part="end" error={errors.end} />
      {!draft.allDay && (
        <Row label="Time zone">
          <select
            aria-label="Time zone"
            className={INPUT}
            disabled={disabled}
            value={draft.timeZone}
            aria-invalid={Boolean(errors.timeZone)}
            aria-describedby={errors.timeZone ? zoneErrorId : undefined}
            onChange={(event) => onChange({ ...draft, timeZone: event.currentTarget.value })}
          >
            {availableZones.map((zone) => (
              <option key={zone} value={zone}>
                {zone}
              </option>
            ))}
          </select>
          {errors.timeZone && (
            <p id={zoneErrorId} className={MESSAGE}>
              {errors.timeZone}
            </p>
          )}
        </Row>
      )}
      <Row label="Repeat">
        <select
          aria-label="Repeat"
          className={INPUT}
          disabled={disabled}
          value={preset}
          onChange={(event) => {
            const value = event.currentTarget.value;
            if (value === "edit") setRepeatOpen(true);
            else if (value !== "custom") changeRule(presetRule(value as Exclude<RepeatPreset, "custom">));
          }}
        >
          {PRESETS.map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
          {preset === "custom" && <option value="custom">{describeRecurrence(`RRULE:${draft.rrule}`, locale)}</option>}
          <option value="edit">Custom…</option>
        </select>
      </Row>
      {draft.rrule && <EndRepeat {...controls} />}
      <CalendarSelect {...controls} calendars={calendars} />
      <Row label="Invitees">
        <InviteesField
          invitees={draft.invitees ?? []}
          onChange={(invitees) => onChange({ ...draft, invitees })}
          suggest={suggest}
          available={Boolean(calendars.find((calendar) => calendar.id === draft.calendarId)?.invites)}
          disabled={disabled}
        />
        {errors.invitees && <p className={MESSAGE}>{errors.invitees}</p>}
      </Row>
      <Alerts {...controls} disabled={readOnly} />
      <Row label="Show as">
        <select
          aria-label="Show as"
          className={INPUT}
          disabled={disabled}
          value={draft.transparent ? "free" : "busy"}
          onChange={(event) => onChange({ ...draft, transparent: event.currentTarget.value === "free" })}
        >
          <option value="busy">Busy</option>
          <option value="free">Free</option>
        </select>
      </Row>
      <textarea
        aria-label="Notes"
        placeholder="Notes"
        className={`${INPUT} min-h-[120px] py-2`}
        disabled={disabled}
        value={draft.description}
        onChange={(event) => onChange({ ...draft, description: event.currentTarget.value })}
      />
      {repeatOpen && !disabled && (
        <RepeatSheet
          initial={repeatOf(draft.rrule, draft.startDate) ?? defaultRepeat(draft.startDate)}
          start={draft.startDate}
          locale={locale}
          onCancel={() => setRepeatOpen(false)}
          onChoose={(repeat) => {
            changeRule(ruleOf(repeat, draft.startDate));
            setRepeatOpen(false);
          }}
        />
      )}
    </div>
  );
}
