// The sidebar's Follow Up section (docs/specs/ui.md, Sidebar): the
// conversations waiting on other people, sent mail with no reply for 3 days
// or more, as maild's thread.awaitingOthers picks them, VIPs first.
import { ChevronDown, Star } from "lucide-react";
import { useState } from "react";

import { displayName } from "../../lib/format";
import type { Awaiting, AwaitingThread } from "../../rpc/gen/api";

/** FOLLOW_UP_DAYS is how long a message waits before the section lists it. */
export const FOLLOW_UP_DAYS = 3;

/** FOLLOW_UP_LIMIT is how many conversations the section asks maild for. */
export const FOLLOW_UP_LIMIT = 25;

/** subjectLabel is a subject, or "(No Subject)" when it is blank, as a list row shows it. */
function subjectLabel(subject: string): string {
  return subject.trim() === "" ? "(No Subject)" : subject;
}

/** toLine names the first two recipients, then counts the rest. */
function toLine(thread: AwaitingThread): string {
  const names = thread.recipients.slice(0, 2).map(displayName).join(", ");
  const more = thread.recipientCount > 2 ? ` & ${thread.recipientCount - 2} more` : "";
  return `To: ${names}${more}`;
}

/** waitingLine says how long the thread's newest message has waited. */
function waitingLine(days: number): string {
  return days === 1 ? "Waiting 1 day" : `Waiting ${days} days`;
}

/** FollowUpSectionProps are the Follow Up section's inputs. */
export interface FollowUpSectionProps {
  /** The conversations waiting on other people; none while they load. */
  awaiting: Awaiting | undefined;
  onOpen: (thread: AwaitingThread) => void;
}

/** FollowUpSection lists the conversations waiting on other people, and hides itself when none wait. */
export function FollowUpSection({ awaiting, onOpen }: FollowUpSectionProps) {
  const [open, setOpen] = useState(true);
  if (awaiting === undefined || awaiting.threads.length === 0) return null;
  return (
    <section aria-label="Follow Up">
      <div className="group flex items-center">
        <button
          type="button"
          aria-expanded={open}
          title={`Sent mail with no reply for ${FOLLOW_UP_DAYS} days or more`}
          onClick={() => setOpen(!open)}
          className="flex h-[26px] min-w-0 flex-1 items-center gap-1 border-0 bg-transparent pl-3 text-left text-sidebar-section text-secondary"
        >
          <span className="truncate">Follow Up</span>
          <ChevronDown size={12} className={`shrink-0 opacity-0 group-hover:opacity-100 ${open ? "" : "-rotate-90"}`} />
        </button>
      </div>
      {open && (
        <>
          <ul>
            {awaiting.threads.map((thread) => (
              <li key={thread.threadId}>
                <button
                  type="button"
                  onClick={() => onOpen(thread)}
                  className="flex w-full flex-col px-4 py-1.5 text-left hover:bg-selection-inactive"
                >
                  <span className="flex min-w-0 items-center gap-1">
                    {thread.vip && <Star size={12} className="shrink-0 text-accent" aria-label="VIP" />}
                    <span className="truncate text-[13px] leading-[18px]">{subjectLabel(thread.message.subject)}</span>
                  </span>
                  <span className="truncate text-[12px] leading-4 text-secondary">{toLine(thread)}</span>
                  <span className="text-[12px] leading-4 text-secondary">{waitingLine(thread.waitingDays)}</span>
                </button>
              </li>
            ))}
          </ul>
          {awaiting.total > awaiting.threads.length && (
            <p className="px-4 py-1.5 text-[12px] leading-4 text-secondary">
              {`Showing ${awaiting.threads.length} of ${awaiting.total}`}
            </p>
          )}
        </>
      )}
    </section>
  );
}
