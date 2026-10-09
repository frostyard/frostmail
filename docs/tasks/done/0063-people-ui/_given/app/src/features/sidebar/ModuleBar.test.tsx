// CONTRACT TEST for task card T-0063 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { ModuleBar } from "./ModuleBar";

describe("ModuleBar", () => {
  it("shows a button per built module, the current one pressed", () => {
    const onSelect = vi.fn();
    render(<ModuleBar modules={["mail", "people"]} current="people" onSelect={onSelect} />);
    const bar = screen.getByRole("toolbar", { name: "Modules" });
    expect(bar.className).toContain("h-[44px]");
    expect(bar.className).toContain("border-separator");
    const buttons = within(bar).getAllByRole("button");
    expect(buttons.map((b) => b.getAttribute("aria-label"))).toEqual(["Mail", "People"]);
    expect(buttons.map((b) => b.getAttribute("title"))).toEqual(["Mail (Ctrl+1)", "People (Ctrl+3)"]);
    expect(buttons.map((b) => b.getAttribute("aria-pressed"))).toEqual(["false", "true"]);
    expect(buttons[1]?.querySelector("svg")?.getAttribute("class")).toContain("text-accent");
    expect(buttons[0]?.querySelector("svg")?.getAttribute("class")).toContain("text-secondary");
    fireEvent.click(buttons[0] as HTMLElement);
    expect(onSelect).toHaveBeenCalledWith("mail");
  });

  it("keeps module order", () => {
    render(<ModuleBar modules={["mail", "calendar", "people", "tasks"]} current="mail" onSelect={vi.fn()} />);
    const names = screen.getAllByRole("button").map((b) => b.getAttribute("aria-label"));
    expect(names).toEqual(["Mail", "Calendar", "People", "Tasks"]);
  });
});
