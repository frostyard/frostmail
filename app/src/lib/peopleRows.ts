// The People list's rows: people with a letter header before each run
// (docs/specs/pim-ui.md, People list). Task T-0063 writes it.
import type { PersonSummary } from "../rpc/gen/api";

/** PeopleRow is a row of the People list. */
export type PeopleRow = { kind: "index"; letter: string } | { kind: "person"; person: PersonSummary };

/** peopleRows puts a header before the first person of each index. */
export function peopleRows(_people: PersonSummary[]): PeopleRow[] {
  return [];
}
