// The reader's invitation card (docs/specs/pim-ui.md, Invitation card).
// Task T-0089 builds it.
import type { Invitation, Part, PartStat } from "../../rpc/gen/api";

/** Answer is what the card's buttons send. */
export type Answer = Extract<PartStat, "accepted" | "tentative" | "declined">;

/** invitationPart is a message's iCalendar part: text/calendar,
 *  application/ics, or a file named *.ics; undefined when it has none. */
export function invitationPart(_parts: Part[]): Part | undefined {
  return undefined;
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

/** InvitationCard shows an invitation and answers it. */
export function InvitationCard(_props: InvitationCardProps) {
  return null;
}
