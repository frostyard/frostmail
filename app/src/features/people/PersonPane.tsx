// The People module's detail pane (docs/specs/pim-ui.md, Person pane).
// Task T-0063 writes it.
import type { MessageSummary, Person } from "../../rpc/gen/api";

/** BookLabel names a contact's address book and account. */
export interface BookLabel {
  name: string;
  account: string;
}

/** PersonPaneProps are the person pane's inputs. */
export interface PersonPaneProps {
  /** null shows "No Contact Selected". */
  person: Person | null;
  /** Address book and account names by collection ID. */
  books: Record<number, BookLabel>;
  /** The person's photo as a data: URL, when loaded. */
  photo?: string;
  /** Recent mail with the person's first email. */
  recent: MessageSummary[];
  /** The clock for the recent mail's dates. */
  now: Date;
  onCompose: (email: string) => void;
  onOpenMessage: (id: number) => void;
  onOpenURL: (url: string) => void;
}

/** PersonPane shows a person's contacts and recent mail. */
export function PersonPane(_props: PersonPaneProps) {
  return null;
}
