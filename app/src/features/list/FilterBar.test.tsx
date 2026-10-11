// CONTRACT TEST for task card T-0101, revised by T-0109 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { emptyText, FilterBar, LIST_FILTERS, type ListFilter } from "./FilterBar";

describe("FilterBar", () => {
  it("offers All, Unread, Flagged, Attachments and More in a 32px strip", () => {
    render(<FilterBar filter="all" onChange={vi.fn()} />);
    const bar = screen.getByRole("toolbar", { name: "Filter messages" });
    expect(bar.className).toContain("h-[32px]");
    expect(bar.className).toContain("border-b");
    expect(bar.className).toContain("border-separator");
    expect(bar.className).toContain("bg-window");
    expect(
      within(bar)
        .getAllByRole("button")
        .map((b) => b.textContent),
    ).toEqual(["All", "Unread", "Flagged", "Attachments", "More"]);
    expect(LIST_FILTERS.map((f) => f.key)).toEqual(["all", "unread", "flagged", "attachments"]);
  });

  it("marks the chosen filter", () => {
    render(<FilterBar filter="flagged" onChange={vi.fn()} />);
    const flagged = screen.getByRole("button", { name: "Flagged" });
    expect(flagged.getAttribute("aria-pressed")).toBe("true");
    expect(flagged.className).toContain("bg-selection-inactive");
    expect(flagged.className).toContain("text-primary");
    expect(flagged.className).toContain("h-[22px]");
    expect(flagged.className).toContain("rounded-md");
    const all = screen.getByRole("button", { name: "All" });
    expect(all.getAttribute("aria-pressed")).toBe("false");
    expect(all.className).toContain("text-secondary");
    expect(all.className).not.toContain("bg-selection-inactive");
  });

  it("reports a click", () => {
    const onChange = vi.fn();
    render(<FilterBar filter="all" onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: "Attachments" }));
    expect(onChange).toHaveBeenCalledWith("attachments");
  });

  it("names the empty list after the filter", () => {
    const filters: ListFilter[] = ["all", "unread", "flagged", "attachments"];
    expect(filters.map(emptyText)).toEqual([
      "No Messages",
      "No Unread Messages",
      "No Flagged Messages",
      "No Messages with Attachments",
    ]);
  });
});
