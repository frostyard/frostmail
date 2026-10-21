import { describe, expect, it } from "vitest";
import { panePercent } from "./panes";

describe("panePercent", () => {
  it("is a pane's share of the window in whole percent", () => {
    expect(panePercent(220, 1200)).toBe(18);
    expect(panePercent(360, 1200)).toBe(30);
    expect(panePercent(100, 100)).toBe(100);
    expect(panePercent(1, 3)).toBe(33);
  });

  it("is 0 when there is no width to divide", () => {
    expect(panePercent(220, 0)).toBe(0);
    expect(panePercent(0, 1200)).toBe(0);
  });
});
