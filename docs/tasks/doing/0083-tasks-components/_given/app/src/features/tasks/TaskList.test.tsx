// CONTRACT TEST for task card T-0083 (docs/tasks). Do not edit.

import { fireEvent, render, screen, within } from "@testing-library/react";
import { createRef } from "react";
import { describe, expect, it, vi } from "vitest";

import { formatListDate } from "../../lib/format";
import { taskLines } from "../../lib/taskText";
import { message, NOW, norm, TASKS, TODAY } from "./fixtures";
import { TaskList, type TaskListProps } from "./TaskList";

const work = TASKS.filter((t) => t.listId === 10);

function list(over: Partial<TaskListProps> = {}) {
  const props: TaskListProps = {
    title: "Work",
    lines: taskLines(work),
    messages: null,
    selected: null,
    focused: false,
    canCreate: true,
    today: TODAY,
    now: NOW,
    locale: "en-US",
    onSelect: vi.fn(),
    onToggle: vi.fn(),
    onCreate: vi.fn(),
    ...over,
  };
  const view = render(<TaskList {...props} />);
  return { props, view };
}

const option = (id: number) => {
  const found = screen.getAllByRole("option").find((o) => o.getAttribute("data-id") === String(id));
  if (!found) throw new Error(`no row ${id}`);
  return found;
};
const circle = (id: number) => within(option(id)).getByRole("checkbox", { name: "Completed" });

describe("TaskList", () => {
  it("shows the tasks in order, a subtask indented under its parent", () => {
    list();
    const box = screen.getByRole("listbox", { name: "Work" });
    expect(
      within(box)
        .getAllByRole("option")
        .map((o) => o.getAttribute("data-id")),
    ).toEqual(["1", "2", "3", "4"]);
    expect(option(1).style.paddingLeft).toBe("12px");
    expect(option(2).style.paddingLeft).toBe("40px");
    expect(option(1).getAttribute("aria-selected")).toBe("false");
  });

  it("shows the due text, the notes' first line and the mail mark", () => {
    list();
    expect(norm(option(1).textContent)).toContain("Report");
    expect(norm(option(1).textContent)).toContain("Tomorrow · Q3 numbers");
    expect(option(1).textContent).not.toContain("and charts");
    expect(option(2).textContent).toBe("Charts");
    expect(within(option(3)).getByText("Yesterday").className).toContain("text-flag-1");
    expect(within(option(1)).getByText("Tomorrow").className).not.toContain("text-flag-1");
    expect(within(option(3)).getByLabelText("From mail")).toBeTruthy();
    expect(within(option(1)).queryByLabelText("From mail")).toBeNull();
  });

  it("ticks with the circle and never selects for it", () => {
    const { props } = list();
    expect(circle(1).getAttribute("aria-checked")).toBe("false");
    expect(circle(4).getAttribute("aria-checked")).toBe("true");
    expect(within(option(4)).getByText("Done").className).toContain("text-tertiary");
    fireEvent.click(circle(1));
    expect(props.onToggle).toHaveBeenLastCalledWith(1, true);
    fireEvent.click(circle(4));
    expect(props.onToggle).toHaveBeenLastCalledWith(4, false);
    expect(props.onSelect).not.toHaveBeenCalled();
  });

  it("disables the circle in a read-only list", () => {
    list({ title: "Shared", lines: taskLines(TASKS.filter((t) => t.listId === 21)), canCreate: false });
    expect((circle(6) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.queryByRole("textbox", { name: "New Task" })).toBeNull();
  });

  it("selects a row on click, in the accent color while focused", () => {
    const { props, view } = list({ selected: 3, focused: true });
    expect(option(3).getAttribute("aria-selected")).toBe("true");
    expect(option(3).className).toContain("bg-accent");
    fireEvent.click(option(1));
    expect(props.onSelect).toHaveBeenLastCalledWith(1);
    view.rerender(<TaskList {...props} selected={3} focused={false} />);
    expect(option(3).className).toContain("bg-selection-inactive");
    expect(option(3).className).not.toContain("bg-accent");
  });

  it("heads each list's tasks in the smart lists", () => {
    const names = new Map([
      [10, "Work"],
      [20, "Home"],
    ]);
    const today = TASKS.filter((t) => t.id === 3 || t.id === 5);
    const { view } = list({ title: "Today", lines: taskLines(today, names) });
    const headers = [...view.container.querySelectorAll("[data-list]")];
    expect(headers.map((h) => [h.getAttribute("data-list"), h.textContent])).toEqual([
      ["10", "Work"],
      ["20", "Home"],
    ]);
    expect(headers[0]?.getAttribute("role")).toBe("presentation");
  });

  it("creates a task from the New Task field and keeps it ready", () => {
    const ref = createRef<HTMLInputElement>();
    const { props } = list({ newTaskRef: ref });
    const field = screen.getByRole("textbox", { name: "New Task" }) as HTMLInputElement;
    expect(ref.current).toBe(field);
    expect(field.placeholder).toBe("New Task");
    fireEvent.change(field, { target: { value: "  Call Ann  " } });
    fireEvent.keyDown(field, { key: "Enter" });
    expect(props.onCreate).toHaveBeenCalledWith("Call Ann");
    expect(field.value).toBe("");
    fireEvent.change(field, { target: { value: "   " } });
    fireEvent.keyDown(field, { key: "Enter" });
    expect(props.onCreate).toHaveBeenCalledTimes(1);
  });

  it("empties the field and focuses the list on Escape", () => {
    list();
    const field = screen.getByRole("textbox", { name: "New Task" }) as HTMLInputElement;
    field.focus();
    fireEvent.change(field, { target: { value: "Draft" } });
    fireEvent.keyDown(field, { key: "Escape" });
    expect(field.value).toBe("");
    expect(document.activeElement).toBe(screen.getByRole("listbox", { name: "Work" }));
  });

  it("says when there are no tasks", () => {
    list({ lines: [] });
    expect(screen.getByText("No Tasks")).toBeTruthy();
    expect(screen.getByRole("textbox", { name: "New Task" })).toBeTruthy();
  });

  it("shows flagged messages with their flags, and ticking clears one", () => {
    const messages = [
      message({ id: 41, subject: "Contract" }),
      message({
        id: 42,
        subject: "",
        from: { name: "", address: "bob@example.com" },
        date: "2026-10-01T09:00:00Z",
        flags: { seen: true, flagged: true, answered: false, forwarded: false, draft: false, flagColor: 2 },
      }),
    ];
    const { props } = list({ title: "Flagged Mail", messages, canCreate: false });
    expect(screen.getByRole("listbox", { name: "Flagged Mail" })).toBeTruthy();
    expect(screen.queryByRole("textbox", { name: "New Task" })).toBeNull();
    expect(screen.getAllByRole("option").map((o) => o.getAttribute("data-id"))).toEqual(["41", "42"]);
    expect(option(41).textContent).toContain("Contract");
    expect(norm(option(41).textContent)).toContain(
      `Ada Lovelace · ${formatListDate(new Date("2026-10-08T10:00:00Z"), NOW, "en-US")}`,
    );
    expect(option(42).textContent).toContain("No Subject");
    expect(option(42).textContent).toContain("bob@example.com");
    expect(within(option(42)).getByLabelText("Flagged Orange").getAttribute("class")).toContain("text-flag-2");
    expect(circle(41).getAttribute("aria-checked")).toBe("false");
    fireEvent.click(circle(41));
    expect(props.onToggle).toHaveBeenCalledWith(41, true);
    expect(props.onSelect).not.toHaveBeenCalled();
  });

  it("says when no mail is flagged", () => {
    list({ title: "Flagged Mail", messages: [], canCreate: false });
    expect(screen.getByText("No Flagged Mail")).toBeTruthy();
  });
});
