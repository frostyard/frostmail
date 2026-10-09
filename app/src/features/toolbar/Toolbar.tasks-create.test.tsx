// CONTRACT TEST for task card T-0091 (docs/tasks). Do not edit.
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { Toolbar, type ToolbarProps, type ToolbarTasks } from "./Toolbar";

function toolbar(tasks: ToolbarTasks) {
  const props: ToolbarProps = {
    mode: "tasks",
    tasks,
    sidebarWidth: 220,
    listWidth: 360,
    title: "",
    subtitle: "",
    syncing: false,
    selection: { count: 0, seen: true, flagColor: 0 },
    canArchive: false,
    moveTargets: [],
    maximized: false,
    search: null,
    onCommand: vi.fn(),
  };
  render(<Toolbar {...props} />);
}

describe("Toolbar's New Task", () => {
  it("is enabled where a task can be made", () => {
    toolbar({ title: "Errands", showCompleted: false, paneWidth: 320 });
    expect((screen.getByRole("button", { name: "New Task" }) as HTMLButtonElement).disabled).toBe(false);
  });

  it("is disabled in Flagged Mail and read-only lists", () => {
    toolbar({ title: "Flagged Mail", showCompleted: null, paneWidth: 320, canCreate: false });
    expect((screen.getByRole("button", { name: "New Task" }) as HTMLButtonElement).disabled).toBe(true);
  });
});
