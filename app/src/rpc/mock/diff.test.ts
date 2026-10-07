import { describe, expect, it } from "vitest";

import { applyOps, diffIds } from "./diff";

function rng(seed: number) {
  let s = seed;
  return () => {
    s = (s * 1103515245 + 12345) & 0x7fffffff;
    return s / 0x7fffffff;
  };
}

describe("diffIds", () => {
  it("produces no ops for equal lists", () => {
    expect(diffIds([1, 2, 3], [1, 2, 3])).toEqual([]);
  });

  it("inserts at the top and removes from the middle", () => {
    expect(diffIds([3, 2, 1], [4, 3, 1])).toEqual([
      { op: "remove", at: 1, count: 1 },
      { op: "insert", at: 0, count: 1 },
    ]);
  });

  it("moves a row by removing and inserting it", () => {
    const prev = [1, 2, 3, 4];
    const next = [3, 1, 2, 4];
    const ops = diffIds(prev, next);
    expect(applyOps(prev, ops, next)).toEqual(next);
    expect(ops.filter((o) => o.op === "remove").reduce((n, o) => n + o.count, 0)).toBe(1);
  });

  it("turns random lists into each other", () => {
    const r = rng(7);
    for (let round = 0; round < 300; round++) {
      const prev = [...Array(Math.floor(r() * 30)).keys()].filter(() => r() < 0.8);
      const next = [...prev.filter(() => r() < 0.8), ...[100, 101, 102].filter(() => r() < 0.5)].sort(() => r() - 0.5);
      expect(applyOps(prev, diffIds(prev, next), next)).toEqual(next);
    }
  });
});
