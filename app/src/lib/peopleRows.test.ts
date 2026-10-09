// CONTRACT TEST for task card T-0063 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import type { PersonSummary } from "../rpc/gen/api";
import { peopleRows } from "./peopleRows";

const p = (id: number, index: string): PersonSummary => ({
  id,
  displayName: `P${id}`,
  organization: "",
  email: "",
  hasPhoto: false,
  index,
});

describe("peopleRows", () => {
  it("puts a header before each run of an index", () => {
    const rows = peopleRows([p(1, "#"), p(2, "A"), p(3, "A"), p(4, "L"), p(5, "A")]);
    expect(rows.map((r) => (r.kind === "index" ? r.letter : r.person.id))).toEqual(["#", 1, "A", 2, 3, "L", 4, "A", 5]);
  });

  it("is empty for no people", () => {
    expect(peopleRows([])).toEqual([]);
  });
});
