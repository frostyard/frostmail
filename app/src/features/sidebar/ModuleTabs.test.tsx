// A narrow window's module tabs (docs/specs/ui.md, Narrow windows).
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { ModuleTabs } from "./ModuleTabs";

describe("ModuleTabs", () => {
  it("names each module under its icon, as touch targets, the current one pressed", () => {
    const onSelect = vi.fn();
    render(<ModuleTabs modules={["mail", "calendar", "people", "tasks"]} current="people" onSelect={onSelect} />);
    const bar = screen.getByRole("toolbar", { name: "Modules" });
    const buttons = within(bar).getAllByRole("button");
    expect(buttons.map((b) => b.textContent)).toEqual(["Mail", "Calendar", "People", "Tasks"]);
    expect(buttons.map((b) => b.getAttribute("aria-pressed"))).toEqual(["false", "false", "true", "false"]);
    for (const b of buttons) expect(b.className).toContain("h-11");
    expect(buttons[2]?.querySelector("svg")?.getAttribute("class")).toContain("text-accent");
    fireEvent.click(within(bar).getByRole("button", { name: "Tasks" }));
    expect(onSelect).toHaveBeenCalledWith("tasks");
  });
});
