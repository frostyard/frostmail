// Details and people for the selected event.
import { Check, CircleHelp, X } from "lucide-react";
import type { ReactNode } from "react";
import { alarmText, answerText, calendarColor, dateText, describeRecurrence, timeRange } from "../../lib/eventText";
import type { Attendee, CalendarEvent } from "../../rpc/gen/api";
import { Avatar } from "../people/Avatar";

/** EventCalendar is the calendar an event is in. */
export interface EventCalendar {
  name: string;
  color: string;
  /** The account's email. */
  account: string;
}

/** EventPaneProps are the event pane's inputs. */
export interface EventPaneProps {
  event: CalendarEvent | null;
  calendar: EventCalendar | null;
  /** The app's time zone. */
  timeZone: string;
  locale: string;
  /** Open the contact card for a person, anchored at their name. */
  onPerson: (email: string, name: string, anchor: HTMLElement) => void;
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt className="text-right text-[12px] leading-4 text-secondary">{label}</dt>
      <dd className="min-w-0 break-words text-[13px] leading-[18px]">{children}</dd>
    </>
  );
}

function People({
  title,
  people,
  onPerson,
}: {
  title: string;
  people: Attendee[];
  onPerson: EventPaneProps["onPerson"];
}) {
  if (people.length === 0) return null;
  return (
    <section>
      <h3 className="mb-2 text-sidebar-section text-secondary">{title}</h3>
      <ul aria-label={title}>
        {people.map((person) => (
          <li key={person.email} className="flex h-9 items-center gap-2 text-[13px] leading-4">
            <Avatar size={28} name={person.name} email={person.email} />
            <div className="min-w-0 flex-1 truncate">
              <button
                type="button"
                className="text-accent hover:underline"
                onClick={(event) => onPerson(person.email, person.name, event.currentTarget)}
              >
                {person.name || person.email}
              </button>
              {person.isUser && " (you)"}
              {person.role === "optional" && <span className="text-tertiary"> (optional)</span>}
            </div>
            {person.answer === "accepted" && <Check size={14} aria-label="Accepted" className="shrink-0 text-flag-4" />}
            {person.answer === "declined" && <X size={14} aria-label="Declined" className="shrink-0 text-flag-1" />}
            {person.answer === "tentative" && (
              <CircleHelp size={14} aria-label="Maybe" className="shrink-0 text-flag-2" />
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

/** EventPane shows the selected event's details. */
export function EventPane({ event, calendar, timeZone, locale, onPerson }: EventPaneProps) {
  if (event === null)
    return (
      <div className="flex h-full items-center justify-center bg-window p-5 text-[13px] leading-4 text-tertiary">
        No Event Selected
      </div>
    );
  return (
    <div className="h-full overflow-y-auto bg-window p-5">
      <header className="mb-4 select-text">
        <h2 className="text-[17px] font-semibold leading-[22px]">{event.summary || "No Title"}</h2>
        {event.location && <div className="text-[13px] leading-4 text-secondary">{event.location}</div>}
        {event.status === "cancelled" && (
          <div className="text-[12px] font-semibold leading-4 text-flag-1">Cancelled</div>
        )}
      </header>
      <div className="mb-5 select-text text-[13px] leading-[18px]">
        <div>{dateText(event, timeZone, locale)}</div>
        <div>{event.allDay ? "All day" : timeRange(event.start, event.end, timeZone, locale)}</div>
        {!event.allDay && event.timeZone && event.timeZone !== timeZone && (
          <div className="text-[12px] leading-4 text-tertiary">
            {event.timeZone}: {timeRange(event.start, event.end, event.timeZone, locale)}
          </div>
        )}
      </div>
      <dl className="mb-5 grid select-text grid-cols-[80px_minmax(0,1fr)] gap-x-3 gap-y-[6px]">
        {event.recurrence && <Field label="Repeats">{describeRecurrence(event.recurrence, locale)}</Field>}
        {calendar && (
          <Field label="Calendar">
            <span
              className="mr-1 inline-block size-2 rounded-full"
              style={{ backgroundColor: calendarColor(calendar.color) }}
            />
            {calendar.name}
            <span className="text-tertiary"> · {calendar.account}</span>
          </Field>
        )}
        {event.alarms.length > 0 && (
          <Field label="Alerts">
            {event.alarms.map((alarm) => (
              <div key={alarm}>{alarmText(alarm)}</div>
            ))}
          </Field>
        )}
        {event.answer && <Field label="Your answer">{answerText(event.answer)}</Field>}
      </dl>
      <div className="space-y-5">
        <People title="Organizer" people={event.organizer ? [event.organizer] : []} onPerson={onPerson} />
        <People title="Invitees" people={event.attendees} onPerson={onPerson} />
        {event.description && (
          <div className="select-text whitespace-pre-wrap text-[13px] leading-[18px]">{event.description}</div>
        )}
      </div>
    </div>
  );
}
