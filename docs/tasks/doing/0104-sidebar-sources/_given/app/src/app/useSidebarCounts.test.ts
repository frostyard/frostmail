// CONTRACT TEST for task card T-0104 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import type { ViewCount } from "../rpc/gen/api";
import { countQueries, toCounts } from "./useSidebarCounts";

const groups = [
  { key: "vip:ann@x.test", label: "Ann", addresses: ["ann@x.test", "ann@home.test"] },
  { key: "vip:bob@y.test", label: "bob@y.test", addresses: ["bob@y.test"] },
];

describe("the sidebar's counts", () => {
  it("ask for VIPs, each group, Flagged and the seven colors, in that order", () => {
    const qs = countQueries(groups);
    expect(qs).toHaveLength(1 + 2 + 1 + 7);
    expect(qs[0]).toEqual({ conditions: { match: "all", conditions: [{ field: "vip", op: "is", value: "true" }] } });
    expect(qs[1]).toEqual({
      conditions: {
        match: "any",
        conditions: [
          { field: "from", op: "is", value: "ann@x.test" },
          { field: "from", op: "is", value: "ann@home.test" },
        ],
      },
    });
    expect(qs[3]).toEqual({ flagged: true });
    expect(qs[4]).toEqual({ conditions: { match: "all", conditions: [{ field: "color", op: "is", value: "1" }] } });
    expect(qs[10]).toEqual({ conditions: { match: "all", conditions: [{ field: "color", op: "is", value: "7" }] } });
  });

  it("map the answers back", () => {
    const answers: ViewCount[] = Array.from({ length: 11 }, (_, i) => ({ total: i, unread: i % 2 }));
    expect(toCounts(groups, answers)).toEqual({
      vips: { total: 0, unread: 0 },
      vip: { "vip:ann@x.test": { total: 1, unread: 1 }, "vip:bob@y.test": { total: 2, unread: 0 } },
      flagged: { total: 3, unread: 1 },
      colors: [4, 5, 6, 7, 8, 9, 10].map((n) => ({ total: n, unread: n % 2 })),
    });
  });
});
