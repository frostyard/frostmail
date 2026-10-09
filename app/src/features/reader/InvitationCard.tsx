// The reader's invitation card (docs/specs/pim-ui.md, Invitation card).
import { TriangleAlert } from "lucide-react";
import { dateText, timeRange } from "../../lib/eventText";
import type { Invitation, Part, PartStat } from "../../rpc/gen/api";

/** Answer is what the card's buttons send. */
export type Answer = Extract<PartStat, "accepted" | "tentative" | "declined">;

/** invitationPart is a message's iCalendar part: text/calendar,
 *  application/ics, or a file named *.ics; undefined when it has none. */
export function invitationPart(parts: Part[]): Part | undefined {
  return (
    parts.find((p) => p.contentType === "text/calendar" || p.contentType === "application/ics") ??
    parts.find((p) => p.filename.toLowerCase().endsWith(".ics"))
  );
}

/** InvitationCardProps are the invitation card's inputs. */
export interface InvitationCardProps {
  invitation: Invitation;
  timeZone: string;
  locale: string;
  /** An answer is on its way: the buttons are disabled. */
  busy: boolean;
  onAnswer: (answer: Answer) => void;
  onShowInCalendar: () => void;
}

const answerWords = (answer: PartStat | undefined) =>
  answer === "accepted" ? "accepted" : answer === "declined" ? "declined" : answer === "tentative" ? "said maybe" : "";

const answers: { answer: Answer; label: string }[] = [
  { answer: "accepted", label: "Accept" },
  { answer: "tentative", label: "Maybe" },
  { answer: "declined", label: "Decline" },
];

/** InvitationCard shows an invitation and answers it. */
export function InvitationCard({
  invitation: inv,
  timeZone,
  locale,
  busy,
  onAnswer,
  onShowInCalendar,
}: InvitationCardProps) {
  const e = inv.event;
  const zone = e.allDay ? "UTC" : timeZone;
  const start = new Date(e.allDay ? e.startDate : e.start);
  const month = new Intl.DateTimeFormat(locale, { timeZone: zone, month: "short" }).format(start).toUpperCase();
  const day = new Intl.DateTimeFormat(locale, { timeZone: zone, day: "numeric" }).format(start);
  const badge =
    inv.method === "request"
      ? "Invitation"
      : inv.method === "cancel"
        ? "Cancelled"
        : inv.method === "reply"
          ? "Reply"
          : "Event";
  const organizer = e.organizer;
  const who =
    inv.method === "reply"
      ? inv.from && `${inv.from.name || inv.from.email} ${answerWords(inv.from.answer)}`.trim()
      : [e.location, organizer && `${organizer.name || organizer.email} (organizer)`].filter(Boolean).join(" · ");
  const adjacent = inv.adjacent
    .map((o, i) => {
      const before = i === 0 && Date.parse(o.end) <= Date.parse(e.start);
      return `${before ? "Before" : "After"}: ${o.summary || "No Title"}`;
    })
    .join(" · ");
  const status = inv.outdated
    ? "This invitation is out of date."
    : answerWords(inv.answer) && `You ${answerWords(inv.answer)}`;
  return (
    <section
      aria-label="Invitation"
      className="mx-3 mt-2 mb-2 flex gap-3 rounded-[8px] border border-separator bg-sidebar p-3"
    >
      <div className="flex h-[44px] w-[40px] shrink-0 flex-col items-center justify-center rounded-[6px] bg-window">
        <span className="text-[10px]/[12px] font-semibold text-flag-1">{month}</span>
        <span className="text-[18px]/[22px] font-semibold">{day}</span>
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline justify-between gap-3">
          <h3 className="text-[15px]/[20px] font-semibold">{e.summary || "No Title"}</h3>
          <span
            className={`text-[11px]/[14px] font-semibold ${inv.method === "cancel" ? "text-flag-1" : "text-secondary"}`}
          >
            {badge}
          </span>
        </div>
        <p className="text-[13px]/[18px]">{`${dateText(e, timeZone, locale)} · ${e.allDay ? "All day" : timeRange(e.start, e.end, timeZone, locale)}`}</p>
        {who && <p className="text-[12px]/[16px] text-secondary">{who}</p>}
        {inv.conflicts.length > 0 && (
          <div className="flex items-center gap-1 text-[12px]/[16px] text-flag-2">
            <TriangleAlert size={12} aria-hidden="true" />
            <span>{`Conflicts with ${inv.conflicts.map((o) => o.summary || "No Title").join(", ")}`}</span>
          </div>
        )}
        {adjacent && <p className="text-[12px]/[16px] text-tertiary">{adjacent}</p>}
        <div className="mt-2 flex items-center justify-between gap-2">
          {inv.canRespond ? (
            // biome-ignore lint/a11y/useSemanticElements: the invitation contract specifies a div answer group.
            <div role="group" aria-label="Answer" className="flex gap-1">
              {answers.map(({ answer, label }) => (
                <button
                  key={answer}
                  type="button"
                  aria-pressed={inv.answer === answer}
                  disabled={busy}
                  onClick={() => onAnswer(answer)}
                  className={`h-[28px] rounded-[6px] border border-separator px-[10px] text-[12px] ${inv.answer === answer ? "bg-accent text-accent-contrast" : "bg-window"}`}
                >
                  {label}
                </button>
              ))}
            </div>
          ) : (
            <p className="text-[12px]/[16px] text-secondary">{status}</p>
          )}
          <button type="button" onClick={onShowInCalendar} className="ml-auto text-[12px] text-accent">
            Show in Calendar
          </button>
        </div>
      </div>
    </section>
  );
}
