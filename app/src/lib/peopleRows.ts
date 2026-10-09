// The People list's rows: people with a letter header before each run
// (docs/specs/pim-ui.md, People list), preserving the supplied order.
import type { PersonSummary } from "../rpc/gen/api";

/** PeopleRow is a row of the People list. */
export type PeopleRow = { kind: "index"; letter: string } | { kind: "person"; person: PersonSummary };

/** peopleRows puts a header before the first person of each index. */
export function peopleRows(people: PersonSummary[]): PeopleRow[] {
  const rows: PeopleRow[] = [];
  let previous: string | undefined;
  for (const person of people) {
    if (person.index !== previous) rows.push({ kind: "index", letter: person.index });
    rows.push({ kind: "person", person });
    previous = person.index;
  }
  return rows;
}
