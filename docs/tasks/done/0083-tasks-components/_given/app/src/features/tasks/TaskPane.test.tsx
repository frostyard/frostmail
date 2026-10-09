// CONTRACT TEST for task card T-0083 (docs/tasks). Do not edit.

import { fireEvent, render, screen } from "@testing-library/react";
import { createRef } from "react";
import { describe, expect, it, vi } from "vitest";

import { formatHeaderDate } from "../../lib/format";
import { message, NOW, norm, TASKS, task } from "./fixtures";
import { TaskPane, type TaskPaneProps } from "./TaskPane";

const report = TASKS[0] ?? task({ id: 1, title: "Report" });
const done = TASKS[3] ?? task({ id: 4, title: "Done" });

function pane(over: Partial<TaskPaneProps> = {}) {
  const props: TaskPaneProps = {
    task: report,
    message: null,
    list: { name: "Work", account: "user@gmail.test" },
    timeZone: "America/New_York",
    locale: "en-US",
    now: NOW,
    onChange: vi.fn(),
    onDelete: vi.fn(),
    onOpenMessage: vi.fn(),
    onClearFlag: vi.fn(),
    ...over,
  };
  const view = render(<TaskPane {...props} />);
  return { props, view };
}

const title = () => screen.getByRole("textbox", { name: "Title" }) as HTMLInputElement;
const notes = () => screen.getByRole("textbox", { name: "Notes" }) as HTMLTextAreaElement;
const due = () => screen.getByLabelText("Due") as HTMLInputElement;
// A row's value: the dd after the dt with its label.
const row = (label: string) => screen.queryByText(label, { selector: "dt" })?.nextElementSibling ?? null;

describe("TaskPane", () => {
  it("says when nothing is selected", () => {
    pane({ task: null, list: null });
    expect(screen.getByText("No Task Selected")).toBeTruthy();
    expect(screen.queryByRole("textbox")).toBeNull();
  });

  it("shows the task's fields", () => {
    pane();
    expect(title().value).toBe("Report");
    expect(due().value).toBe("2026-10-10");
    expect(due().type).toBe("date");
    expect(notes().value).toBe("Q3 numbers\nand charts");
    expect(norm(row("List")?.textContent)).toBe("Work · user@gmail.test");
    expect(row("Completed")).toBeNull();
    expect(screen.queryByRole("button", { name: "Open Message" })).toBeNull();
  });

  it("commits a changed title on Enter and on leaving, and nothing unchanged", () => {
    const { props } = pane();
    fireEvent.blur(title());
    expect(props.onChange).not.toHaveBeenCalled();
    fireEvent.change(title(), { target: { value: "Report v2" } });
    fireEvent.keyDown(title(), { key: "Enter" });
    expect(props.onChange).toHaveBeenLastCalledWith(1, { title: "Report v2" });
    fireEvent.change(title(), { target: { value: "Report v3 " } });
    fireEvent.blur(title());
    expect(props.onChange).toHaveBeenLastCalledWith(1, { title: "Report v3" });
  });

  it("returns to the old title when emptied or on Escape", () => {
    const { props } = pane();
    fireEvent.change(title(), { target: { value: "  " } });
    fireEvent.blur(title());
    expect(title().value).toBe("Report");
    fireEvent.change(title(), { target: { value: "Nope" } });
    fireEvent.keyDown(title(), { key: "Escape" });
    expect(title().value).toBe("Report");
    expect(props.onChange).not.toHaveBeenCalled();
  });

  it("starts over when another task is selected", () => {
    const { props, view } = pane();
    fireEvent.change(title(), { target: { value: "Half typed" } });
    view.rerender(<TaskPane {...props} task={TASKS[2] ?? report} />);
    expect(title().value).toBe("Groceries");
  });

  it("commits the due date at once, and clears it", () => {
    const { props } = pane();
    fireEvent.change(due(), { target: { value: "2026-10-12" } });
    expect(props.onChange).toHaveBeenLastCalledWith(1, { due: "2026-10-12" });
    fireEvent.click(screen.getByRole("button", { name: "Clear Due Date" }));
    expect(props.onChange).toHaveBeenLastCalledWith(1, { due: "" });
  });

  it("has no Clear without a due date", () => {
    pane({ task: TASKS[1] ?? report });
    expect(due().value).toBe("");
    expect(screen.queryByRole("button", { name: "Clear Due Date" })).toBeNull();
  });

  it("commits changed notes on leaving", () => {
    const { props } = pane();
    fireEvent.blur(notes());
    expect(props.onChange).not.toHaveBeenCalled();
    fireEvent.change(notes(), { target: { value: "Q4 numbers" } });
    fireEvent.blur(notes());
    expect(props.onChange).toHaveBeenLastCalledWith(1, { notes: "Q4 numbers" });
  });

  it("shows when a task was completed, in the app's zone", () => {
    pane({ task: done });
    expect(norm(row("Completed")?.textContent)).toBe("Oct 9, 2026, 3:04 PM");
  });

  it("opens the message a task was made from", () => {
    const { props } = pane({ task: TASKS[2] ?? report });
    fireEvent.click(screen.getByRole("button", { name: "Open Message" }));
    expect(props.onOpenMessage).toHaveBeenCalledWith(77);
  });

  it("deletes the task", () => {
    const { props } = pane();
    fireEvent.click(screen.getByRole("button", { name: "Delete Task" }));
    expect(props.onDelete).toHaveBeenCalledWith(1);
  });

  it("lets the container focus the title", () => {
    const ref = createRef<HTMLInputElement>();
    pane({ titleRef: ref });
    expect(ref.current).toBe(title());
  });

  it("changes nothing in a read-only list", () => {
    pane({ task: TASKS[5] ?? report, list: { name: "Shared", account: "user@dav.test" } });
    expect(title().readOnly).toBe(true);
    expect(notes().readOnly).toBe(true);
    expect(due().disabled).toBe(true);
    expect(screen.getByText("Read-only")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Delete Task" })).toBeNull();
  });

  it("shows a flagged message with Open in Mail and Clear Flag", () => {
    const { props } = pane({ task: null, list: null, message: message({ id: 41, subject: "Contract" }) });
    const heading = screen.getByRole("heading", { level: 2 });
    expect(heading.textContent).toBe("Contract");
    const sent = formatHeaderDate(new Date("2026-10-08T10:00:00Z"), "en-US");
    expect(norm(heading.nextElementSibling?.textContent)).toBe(norm(`Ada Lovelace · ${sent}`));
    fireEvent.click(screen.getByRole("button", { name: "Open in Mail" }));
    expect(props.onOpenMessage).toHaveBeenCalledWith(41);
    fireEvent.click(screen.getByRole("button", { name: "Clear Flag" }));
    expect(props.onClearFlag).toHaveBeenCalledWith(41);
    expect(screen.queryByText("No Task Selected")).toBeNull();
  });
});
