// lodge-ui ui/v0.7.0 darkened the light --text-tertiary (#6c6c71) and
// lightened the dark one (#95959a): on --accent it reaches only 1.26:1 and
// 1.33:1. A selected row in a focused list is accent, so a completed task's
// title takes the row's contrast colour, not tertiary.
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { taskLines } from "../../lib/taskText";
import { NOW, TASKS, TODAY } from "./fixtures";
import { TaskList } from "./TaskList";

const work = TASKS.filter((t) => t.listId === 10);

function title(focused: boolean) {
  render(
    <TaskList
      title="Work"
      lines={taskLines(work)}
      messages={null}
      selected={4}
      focused={focused}
      canCreate
      today={TODAY}
      now={NOW}
      locale="en-US"
      onSelect={vi.fn()}
      onToggle={vi.fn()}
      onCreate={vi.fn()}
    />,
  );
  const row = screen.getAllByRole("option").find((o) => o.getAttribute("data-id") === "4");
  if (!row) throw new Error("no row 4");
  expect(row.getAttribute("aria-checked")).toBe("true");
  return { row, className: within(row).getByText("Done").className };
}

describe("TaskList's completed title on the accent row", () => {
  it("is not tertiary on the selected row of a focused list", () => {
    const { row, className } = title(true);
    expect(row.className).toContain("text-accent-contrast");
    expect(className).not.toContain("text-tertiary");
  });

  it("stays tertiary on the selected row of an unfocused list", () => {
    expect(title(false).className).toContain("text-tertiary");
  });
});
