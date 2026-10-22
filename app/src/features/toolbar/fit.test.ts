import { describe, expect, it } from "vitest";

import { fitItems } from "./fit";

/** widths is n items of one width, as both the toolbar and the bars have. */
function widths(count: number, width: number): number[] {
  return Array.from({ length: count }, () => width);
}

describe("fitItems", () => {
  it("shows every item before layout", () => {
    expect(fitItems(0, widths(8, 28), 28, 4)).toBe(8);
    expect(fitItems(-1, widths(8, 28), 28, 4)).toBe(8);
  });

  it("shows every item when they all fit with their own gaps and no »", () => {
    expect(fitItems(252, widths(8, 28), 28, 4)).toBe(8);
    expect(fitItems(380, widths(8, 44), 44, 4)).toBe(8);
  });

  it("fits an exact boundary, and one hundredth less does not", () => {
    // Three 28px items and the » with a gap between each: 84 + 28 + 12 = 124.
    expect(fitItems(124, widths(8, 28), 28, 4)).toBe(3);
    expect(fitItems(123.99, widths(8, 28), 28, 4)).toBe(2);
  });

  it("shows 8 of the message bar's 44px actions at 390px (382 available)", () => {
    expect(fitItems(382, widths(8, 44), 44, 4)).toBe(8);
  });

  it("shows 5 of them at 320px (312 available)", () => {
    expect(fitItems(312, widths(8, 44), 44, 4)).toBe(5);
  });

  it("shows 2 of the reader's 28px actions in an 860px window (96 available)", () => {
    expect(fitItems(96, widths(8, 28), 28, 4)).toBe(2);
  });

  it("shows none when even the » button does not fit", () => {
    expect(fitItems(27, widths(8, 28), 28, 4)).toBe(0);
    expect(fitItems(27, widths(8, 44), 44, 4)).toBe(0);
  });
});
