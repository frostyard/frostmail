// CONTRACT TEST for task card T-0083 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { type TaskListSection, TasksSidebar, type TasksSidebarProps } from "./TasksSidebar";

const sections: TaskListSection[] = [
  { accountId: 1, title: "user@gmail.test", lists: [{ id: 10, name: "Work", readOnly: false, count: 3 }] },
  {
    accountId: 2,
    title: "user@dav.test",
    lists: [
      { id: 20, name: "Home", readOnly: false, count: 0 },
      { id: 21, name: "Shared", readOnly: true, count: 1 },
    ],
  },
  { accountId: 3, title: "empty@dav.test", lists: [] },
];

function sidebar(over: Partial<TasksSidebarProps> = {}) {
  const props: TasksSidebarProps = {
    sections,
    counts: { today: 2, all: 5, flagged: 0 },
    selected: "today",
    focused: false,
    onSelect: vi.fn(),
    ...over,
  };
  render(<TasksSidebar {...props} />);
  return props;
}

const row = (name: string) => screen.getByRole("treeitem", { name: new RegExp(`^${name}`) });
const count = (name: string) => row(name).querySelector("[data-count]")?.textContent ?? null;

describe("TasksSidebar", () => {
  it("lists the smart lists, then each account's task lists", () => {
    sidebar();
    const tree = screen.getByRole("tree", { name: "Task Lists" });
    expect(tree.getAttribute("tabindex")).toBe("0");
    expect(
      within(tree)
        .getAllByRole("treeitem")
        .map((r) => r.getAttribute("data-key")),
    ).toEqual(["today", "all", "flagged", "list:10", "list:20", "list:21"]);
    for (const title of ["Tasks", "user@gmail.test", "user@dav.test"]) {
      expect(screen.getByRole("button", { name: title }).getAttribute("aria-expanded")).toBe("true");
    }
    expect(screen.queryByRole("button", { name: "empty@dav.test" })).toBeNull();
    expect(row("All Tasks").textContent).toContain("All Tasks");
    expect(row("Flagged Mail").textContent).toBe("Flagged Mail");
  });

  it("shows counts but not zeros", () => {
    sidebar();
    expect(count("Today")).toBe("2");
    expect(count("All Tasks")).toBe("5");
    expect(count("Flagged Mail")).toBeNull();
    expect(count("Work")).toBe("3");
    expect(count("Home")).toBeNull();
    expect(count("Shared")).toBe("1");
  });

  it("marks read-only lists", () => {
    sidebar();
    expect(within(row("Shared")).getByLabelText("Read-only")).toBeTruthy();
    expect(within(row("Work")).queryByLabelText("Read-only")).toBeNull();
  });

  it("shows the selection in the accent color while focused, gray otherwise", () => {
    sidebar({ selected: 10, focused: true });
    expect(row("Work").getAttribute("aria-selected")).toBe("true");
    expect(row("Work").className).toContain("bg-accent");
    expect(row("Work").querySelector("[data-count]")?.className).toContain("text-accent-contrast");
    expect(row("Today").getAttribute("aria-selected")).toBe("false");
  });

  it("shows an unfocused selection in the sidebar's gray", () => {
    sidebar({ selected: "flagged" });
    expect(row("Flagged Mail").className).toContain("bg-selection-sidebar");
    expect(row("Flagged Mail").className).not.toContain("bg-accent");
  });

  it("selects a row on click", () => {
    const props = sidebar();
    fireEvent.click(row("Home"));
    expect(props.onSelect).toHaveBeenLastCalledWith(20);
    fireEvent.click(row("Flagged Mail"));
    expect(props.onSelect).toHaveBeenLastCalledWith("flagged");
  });

  it("moves the selection with the arrows, Home and End", () => {
    const props = sidebar({ selected: 10 });
    const tree = screen.getByRole("tree", { name: "Task Lists" });
    fireEvent.keyDown(tree, { key: "ArrowUp" });
    expect(props.onSelect).toHaveBeenLastCalledWith("flagged");
    fireEvent.keyDown(tree, { key: "ArrowDown" });
    expect(props.onSelect).toHaveBeenLastCalledWith(20);
    fireEvent.keyDown(tree, { key: "Home" });
    expect(props.onSelect).toHaveBeenLastCalledWith("today");
    fireEvent.keyDown(tree, { key: "End" });
    expect(props.onSelect).toHaveBeenLastCalledWith(21);
  });

  it("collapses a section and skips its rows", () => {
    const props = sidebar({ selected: 10 });
    fireEvent.click(screen.getByRole("button", { name: "user@dav.test" }));
    expect(screen.getByRole("button", { name: "user@dav.test" }).getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("treeitem", { name: /^Home/ })).toBeNull();
    fireEvent.keyDown(screen.getByRole("tree", { name: "Task Lists" }), { key: "ArrowDown" });
    expect(props.onSelect).not.toHaveBeenCalled();
  });
});
