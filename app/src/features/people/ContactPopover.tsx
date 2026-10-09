// The contact card mail shows for an address (docs/specs/pim-ui.md,
// Contact card). Task T-0065 writes it.
import type { Address, ContactCard } from "../../rpc/gen/api";

/** AddState is where Add to Contacts stands. */
export type AddState = "idle" | "adding" | "added" | { error: string };

/** ContactPopoverProps are the contact card's inputs. */
export interface ContactPopoverProps {
  /** The address the card was opened for; shown until card arrives. */
  address: Address;
  /** people.card's answer, or null while it is pending. */
  card: ContactCard | null;
  /** The person's photo as a data: URL, when loaded. */
  photo?: string;
  /** Where the card goes: below the clicked name, in window pixels. */
  at: { x: number; y: number };
  /** The clock for the recent mail's dates. */
  now: Date;
  add: AddState;
  onCompose: () => void;
  onAdd: () => void;
  onOpenPerson: (id: number) => void;
  onOpenMessage: (id: number) => void;
  onClose: () => void;
}

/** ContactPopover shows who an address is and what to do with them. */
export function ContactPopover(_props: ContactPopoverProps) {
  return null;
}
