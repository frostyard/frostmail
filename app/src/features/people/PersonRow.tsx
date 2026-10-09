// A row of the People list and the list's letter headers
// (docs/specs/pim-ui.md, People list). Task T-0063 writes them.
import type { PersonSummary } from "../../rpc/gen/api";

/** PersonRowProps are a row's inputs. */
export interface PersonRowProps {
  person: PersonSummary;
  selected: boolean;
  /** The list has keyboard focus: the selection uses the accent color. */
  focused: boolean;
  onSelect: (id: number) => void;
}

/** PersonRow shows a person's avatar, name and organization or email. */
export function PersonRow(_props: PersonRowProps) {
  return null;
}

/** IndexHeader is the letter heading a run of people in the list. */
export function IndexHeader(_props: { letter: string }) {
  return null;
}
