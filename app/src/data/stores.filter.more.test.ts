// CONTRACT TEST for task card T-0109 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { listQuery } from "./stores";

const filter = (field: string) => ({ match: "all", conditions: [{ field, op: "is", value: "true" }] });

describe("the More filters' queries", () => {
  it("add a filter beside the source's own query", () => {
    const base = {
      source: { kind: "allInboxes" } as const,
      search: "",
      searchScope: "all" as const,
      conversations: true,
    };
    expect(listQuery({ ...base, listFilter: "toMe" })).toEqual({
      role: "inbox",
      threads: true,
      filter: filter("tome"),
    });
    expect(listQuery({ ...base, listFilter: "ccMe" })).toEqual({
      role: "inbox",
      threads: true,
      filter: filter("ccme"),
    });
    expect(
      listQuery({
        ...base,
        source: { kind: "vip", key: "vip:a@x.test", addresses: ["a@x.test", "a@y.test"] },
        listFilter: "vips",
      }),
    ).toEqual({
      conditions: {
        match: "any",
        conditions: [
          { field: "from", op: "is", value: "a@x.test" },
          { field: "from", op: "is", value: "a@y.test" },
        ],
      },
      threads: true,
      filter: filter("vip"),
    });
  });

  it("narrow a search everywhere", () => {
    expect(
      listQuery({
        source: { kind: "allInboxes" },
        search: "lunch",
        searchScope: "all",
        conversations: false,
        listFilter: "toMe",
      }),
    ).toEqual({ text: "lunch", filter: filter("tome") });
  });
});
