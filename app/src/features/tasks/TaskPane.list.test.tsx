// CONTRACT TEST for task card T-0091 (docs/tasks). Do not edit.
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { NOW, TASKS, task } from "./fixtures";
import { TaskPane } from "./TaskPane";

describe("TaskPane's List row", () => {
  it("keeps the list and account on one truncated line, the whole in its title", () => {
    render(
      <TaskPane
        task={TASKS[0] ?? task({ id: 1, title: "Report" })}
        message={null}
        list={{ name: "Work", account: "someone.with.a.long.address@northwind.example" }}
        timeZone="UTC"
        locale="en-US"
        now={NOW}
        onChange={vi.fn()}
        onDelete={vi.fn()}
        onOpenMessage={vi.fn()}
        onClearFlag={vi.fn()}
      />,
    );
    const dd = screen.getByText("List", { selector: "dt" }).nextElementSibling;
    const line = dd?.firstElementChild;
    expect(line?.className).toContain("truncate");
    expect(line?.getAttribute("title")).toBe("Work · someone.with.a.long.address@northwind.example");
  });
});
