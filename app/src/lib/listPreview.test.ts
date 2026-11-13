import { describe, expect, it } from "vitest";

import type { Settings } from "../rpc/gen/api";
import { previewLines, rowHeight } from "./listPreview";

const settings = (previewLines?: number): Settings => ({
  undoDelay: 10,
  notifyScope: "inbox",
  flagNames: ["", "", "", "", "", "", ""],
  ...(previewLines === undefined ? {} : { previewLines }),
});

describe("previewLines", () => {
  it("is the user's choice, 0 to 5", () => {
    for (const n of [0, 1, 2, 3, 4, 5]) expect(previewLines(settings(n))).toBe(n);
  });

  it("is 2 before the settings load, from an older maild, and for a value out of range", () => {
    expect(previewLines(null)).toBe(2);
    expect(previewLines(undefined)).toBe(2);
    expect(previewLines(settings())).toBe(2);
    for (const n of [-1, 6, 2.5, Number.NaN]) expect(previewLines(settings(n))).toBe(2);
  });
});

describe("rowHeight", () => {
  it("is 84 for 2 lines and 16 more or less for each other", () => {
    expect([0, 1, 2, 3, 4, 5].map(rowHeight)).toEqual([52, 68, 84, 100, 116, 132]);
    expect(rowHeight(-2)).toBe(52);
  });
});
