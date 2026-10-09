// CONTRACT TEST for task card T-0084 (docs/tasks). Do not edit.
import { beforeEach, describe, expect, it } from "vitest";

import { useUI } from "./stores";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

describe("tasks UI state", () => {
  it("starts on Today with nothing selected and completed tasks hidden", () => {
    const ui = useUI.getState();
    expect(ui.tasksSource).toBe("today");
    expect(ui.tasksSelected).toBeNull();
    expect(ui.tasksFocus).toBe("list");
    expect(ui.tasksShowCompleted).toBe(false);
    expect(ui.todoBar).toBe(false);
  });

  it("selects a task, and a new source clears the selection", () => {
    const ui = useUI.getState();
    ui.selectTask(7);
    expect(useUI.getState().tasksSelected).toBe(7);
    ui.setTasksSource(301);
    expect(useUI.getState().tasksSource).toBe(301);
    expect(useUI.getState().tasksSelected).toBeNull();
    ui.selectTask(8);
    ui.setTasksSource("flagged");
    expect([useUI.getState().tasksSource, useUI.getState().tasksSelected]).toEqual(["flagged", null]);
  });

  it("toggles Show Completed and moves focus without losing the selection", () => {
    const ui = useUI.getState();
    ui.selectTask(7);
    ui.toggleShowCompleted();
    ui.setTasksFocus("reader");
    const after = useUI.getState();
    expect([after.tasksShowCompleted, after.tasksFocus, after.tasksSelected]).toEqual([true, "reader", 7]);
    after.toggleShowCompleted();
    expect(useUI.getState().tasksShowCompleted).toBe(false);
  });

  it("persists the To-Do bar but not the Tasks module's state", () => {
    const ui = useUI.getState();
    ui.toggleTodoBar();
    ui.setTasksSource(301);
    ui.toggleShowCompleted();
    expect(useUI.getState().todoBar).toBe(true);
    const saved = JSON.parse(localStorage.getItem("frostmail.ui") ?? "{}") as { state?: Record<string, unknown> };
    expect(saved.state?.todoBar).toBe(true);
    expect(saved.state?.tasksSource).toBeUndefined();
    expect(saved.state?.tasksShowCompleted).toBeUndefined();
    useUI.getState().toggleTodoBar();
    expect(useUI.getState().todoBar).toBe(false);
  });
});
