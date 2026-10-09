// Opens the contact card for an address in the reader (docs/specs/pim-ui.md,
// Contact card). Task T-0065 writes it.
import type { Address } from "../rpc/gen/api";

/** ContactCardContainerProps say which card is open. */
export interface ContactCardContainerProps {
  address: Address;
  at: { x: number; y: number };
  onClose: () => void;
}

/** ContactCardContainer loads a contact card and carries out its actions. */
export function ContactCardContainer(_props: ContactCardContainerProps) {
  return null;
}
