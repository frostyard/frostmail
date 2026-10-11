// CONTRACT TEST for task card T-0109 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { emptyText, FilterBar, MORE_FILTERS } from "./FilterBar";

describe("the filter bar's More menu", () => {
  it("offers To Me, Cc Me and From VIPs", () => {
    const onChange = vi.fn();
    render(<FilterBar filter="all" onChange={onChange} />);
    expect(MORE_FILTERS).toEqual([
      { key: "toMe", label: "To Me" },
      { key: "ccMe", label: "Cc Me" },
      { key: "vips", label: "From VIPs" },
    ]);
    const more = screen.getByRole("button", { name: "More filters" });
    expect(more.getAttribute("aria-haspopup")).toBe("menu");
    expect(more.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(more);
    const items = screen.getAllByRole("menuitemcheckbox");
    expect(items.map((i) => i.textContent)).toEqual(["To Me", "Cc Me", "From VIPs"]);
    fireEvent.click(items[1] as HTMLElement);
    expect(onChange).toHaveBeenCalledWith("ccMe");
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("shows the chosen one, pressed", () => {
    render(<FilterBar filter="vips" onChange={vi.fn()} />);
    const more = screen.getByRole("button", { name: "More filters" });
    expect(more.textContent).toBe("From VIPs");
    expect(more.getAttribute("aria-pressed")).toBe("true");
    expect(more.className).toContain("bg-selection-inactive");
    fireEvent.click(more);
    expect(screen.getByRole("menuitemcheckbox", { name: /From VIPs/ }).getAttribute("aria-checked")).toBe("true");
  });

  it("names their empty lists", () => {
    expect(["toMe", "ccMe", "vips"].map((f) => emptyText(f as Parameters<typeof emptyText>[0]))).toEqual([
      "No Messages to You",
      "No Messages Cc'd to You",
      "No Messages from VIPs",
    ]);
  });
});
