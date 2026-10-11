// CONTRACT TEST for task card T-0104 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import type { Vip } from "../rpc/gen/api";
import { sourceFromKey, sourceKey, sourceQuery } from "./stores";

const vips: Vip[] = [
  { address: "ann@x.test", name: "Ann Smith", personId: 7 },
  { address: "ann@home.test", name: "Ann Smith", personId: 7 },
  { address: "bob@y.test", name: "" },
];

describe("the built-in sources", () => {
  it("parse from and print to their sidebar keys", () => {
    for (const key of ["vips", "flag:1", "flag:7", "vip:ann@x.test", "vip:bob@y.test", "flagged", "all-inboxes"]) {
      const source = sourceFromKey(key, vips);
      expect(source).not.toBeNull();
      if (source) expect(sourceKey(source)).toBe(key);
    }
    expect(sourceFromKey("vip:ann@x.test", vips)).toEqual({
      kind: "vip",
      key: "vip:ann@x.test",
      addresses: ["ann@x.test", "ann@home.test"],
    });
    expect(sourceFromKey("flag:3")).toEqual({ kind: "flagColor", color: 3 });
    expect(sourceFromKey("vips")).toEqual({ kind: "vips" });
    for (const key of ["flag:0", "flag:8", "flag:x", "vip:nobody@x.test", "vip:"]) {
      expect(sourceFromKey(key, vips)).toBeNull();
    }
  });

  it("list by conditions", () => {
    expect(sourceQuery({ kind: "flagColor", color: 2 }, true)).toEqual({
      conditions: { match: "all", conditions: [{ field: "color", op: "is", value: "2" }] },
      threads: true,
    });
    expect(sourceQuery({ kind: "vips" }, false)).toEqual({
      conditions: { match: "all", conditions: [{ field: "vip", op: "is", value: "true" }] },
    });
    expect(
      sourceQuery({ kind: "vip", key: "vip:ann@x.test", addresses: ["ann@x.test", "ann@home.test"] }, true),
    ).toEqual({
      conditions: {
        match: "any",
        conditions: [
          { field: "from", op: "is", value: "ann@x.test" },
          { field: "from", op: "is", value: "ann@home.test" },
        ],
      },
      threads: true,
    });
  });
});
