// happy-dom has no layout. Supply the natural button widths and the space
// left by Sort, then drive the same ResizeObserver as the real toolbar.
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { FilterBar, fitFilters } from "./FilterBar";

// Chromium, Inter Variable 12/16: a 338.015625px natural toolbar, including
// 24px padding and four 4px gaps. Keep fractions to test the fit boundary.
const TAB_WIDTHS = [29.875, 57.796875, 60.96875, 87.734375];
const MORE_WIDTHS: Record<string, number> = {
  More: 61.640625,
  All: 45.875,
  Unread: 73.796875,
  Flagged: 76.96875,
  Attachments: 103.734375,
  "To Me": 65.875,
  "Cc Me": 66.125,
  "From VIPs": 88.828125,
};
const moreWidth = (label: string) => MORE_WIDTHS[label] ?? 0;
const SORT_WIDTH = 102.34375;
// The row ends with 12px padding. The filter bar has 24px padding.
const available = (windowWidth: number) => windowWidth - SORT_WIDTH - 12 - 24;

function layout(initialWidth: number) {
  let width = initialWidth;
  const observers: (() => void)[] = [];
  const disconnect = vi.fn();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: () => void) {
        observers.push(callback);
      }
      observe() {}
      disconnect = disconnect;
    },
  );
  const computedStyle = window.getComputedStyle.bind(window);
  vi.spyOn(window, "getComputedStyle").mockImplementation((element) => {
    const style = computedStyle(element);
    if (element.getAttribute("aria-label") === "Filter messages") {
      const measured = document.createElement("div").style;
      measured.cssText = style.cssText;
      measured.paddingLeft = "12px";
      measured.paddingRight = "12px";
      measured.columnGap = "4px";
      return measured;
    }
    return style;
  });
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    if (this.getAttribute("aria-label") === "Filter messages") return new DOMRect(0, 0, width - SORT_WIDTH - 12, 32);
    const label = this.textContent ?? "";
    const tab = ["All", "Unread", "Flagged", "Attachments"].indexOf(label);
    const naturalWidth = this.querySelector("svg") ? moreWidth(label) : (TAB_WIDTHS[tab] ?? 0);
    return new DOMRect(0, 0, naturalWidth, 22);
  });
  return {
    disconnect,
    resize(nextWidth: number) {
      width = nextWidth;
      act(() => {
        for (const notify of observers) notify();
      });
    },
  };
}

function buttons() {
  return within(screen.getByRole("toolbar", { name: "Filter messages" }))
    .getAllByRole("button")
    .map((button) => button.textContent);
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("fitFilters", () => {
  it("keeps All, Unread and Flagged at 390px, and drops Flagged at 320px", () => {
    expect(fitFilters(available(390), TAB_WIDTHS, moreWidth, 4)).toBe(3);
    expect(fitFilters(available(320), TAB_WIDTHS, moreWidth, 4)).toBe(2);
  });

  it("shows all tabs at a wide width and before layout", () => {
    expect(fitFilters(available(600), TAB_WIDTHS, moreWidth, 4)).toBe(4);
    expect(fitFilters(0, TAB_WIDTHS, moreWidth, 4)).toBe(4);
  });

  it("reserves the chosen menu label, not just More", () => {
    expect(fitFilters(available(390), TAB_WIDTHS, moreWidth, 4, "attachments")).toBe(2);
    expect(fitFilters(available(320), TAB_WIDTHS, moreWidth, 4, "vips")).toBe(1);
    expect(fitFilters(available(600), TAB_WIDTHS, moreWidth, 4, "attachments")).toBe(4);
  });

  it("fits an exact boundary, and always keeps All", () => {
    const exact = TAB_WIDTHS.slice(0, 3).reduce((sum, width) => sum + width, 0) + moreWidth("More") + 3 * 4;
    expect(fitFilters(exact, TAB_WIDTHS, moreWidth, 4)).toBe(3);
    expect(fitFilters(exact - 0.5, TAB_WIDTHS, moreWidth, 4)).toBe(2);
    expect(fitFilters(1, TAB_WIDTHS, moreWidth, 4)).toBe(1);
  });
});

describe("the measured filter bar", () => {
  it("moves Attachments before the additional filters and reports its selection", () => {
    layout(390);
    const onChange = vi.fn();
    const { rerender } = render(<FilterBar filter="all" onChange={onChange} />);
    expect(buttons()).toEqual(["All", "Unread", "Flagged", "More"]);
    fireEvent.click(screen.getByRole("button", { name: "More filters" }));
    const menu = screen.getByRole("menu");
    const entries = within(menu).getAllByRole("menuitemcheckbox");
    expect(entries.map((item) => item.textContent)).toEqual(["Attachments", "To Me", "Cc Me", "From VIPs"]);
    expect(within(menu).getAllByRole("separator")).toHaveLength(1);
    const separator = within(menu).getByRole("separator");
    expect((entries[0]?.compareDocumentPosition(separator) ?? 0) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect((entries[1]?.compareDocumentPosition(separator) ?? 0) & Node.DOCUMENT_POSITION_PRECEDING).toBeTruthy();
    fireEvent.click(within(menu).getByRole("menuitemcheckbox", { name: "Attachments" }));
    expect(onChange).toHaveBeenCalledWith("attachments");
    expect(screen.queryByRole("menu")).toBeNull();

    rerender(<FilterBar filter="attachments" onChange={onChange} />);
    const more = screen.getByRole("button", { name: "More filters" });
    expect(more.textContent).toBe("Attachments");
    expect(more.getAttribute("aria-pressed")).toBe("true");
    expect(buttons()).toEqual(["All", "Unread", "Attachments"]);
    fireEvent.click(more);
    expect(screen.getByRole("menuitemcheckbox", { name: "Attachments" }).getAttribute("aria-checked")).toBe("true");
  });

  it("refits on resize and puts a chosen tab back in the bar", () => {
    const sizes = layout(390);
    const { rerender, unmount } = render(<FilterBar filter="flagged" onChange={vi.fn()} />);
    sizes.resize(320);
    expect(buttons()).toEqual(["All", "Unread", "Flagged"]);
    expect(screen.getByRole("button", { name: "More filters" }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: "More filters" }));
    expect(screen.getAllByRole("menuitemcheckbox").map((item) => item.textContent)).toEqual([
      "Flagged",
      "Attachments",
      "To Me",
      "Cc Me",
      "From VIPs",
    ]);
    fireEvent.keyDown(document, { key: "Escape" });
    sizes.resize(600);
    expect(buttons()).toEqual(["All", "Unread", "Flagged", "Attachments", "More"]);
    expect(screen.getByRole("button", { name: "More filters" }).getAttribute("aria-pressed")).toBe("false");
    rerender(<FilterBar filter="all" onChange={vi.fn()} />);
    sizes.resize(320);
    expect(buttons()).toEqual(["All", "Unread", "More"]);
    unmount();
    expect(sizes.disconnect).toHaveBeenCalledOnce();
  });
});
