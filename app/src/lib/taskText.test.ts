// CONTRACT TEST for task card T-0083 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { TASKS, TODAY, task } from "../features/tasks/fixtures";
import { dueSoon, dueText, isOverdue, openCount, sourceTasks, taskLines } from "./taskText";

const ids = (tasks: { id: number }[]) => tasks.map((t) => t.id);
const none = new Set<number>();

describe("dueText", () => {
  it("names the days around today", () => {
    expect(dueText("2026-10-09", TODAY, "en-US")).toBe("Today");
    expect(dueText("2026-10-10", TODAY, "en-US")).toBe("Tomorrow");
    expect(dueText("2026-10-08", TODAY, "en-US")).toBe("Yesterday");
    expect(dueText("", TODAY, "en-US")).toBe("");
  });

  it("gives other days their weekday in today's year, and the year otherwise", () => {
    expect(dueText("2026-10-16", TODAY, "en-US")).toBe("Fri, Oct 16");
    expect(dueText("2026-01-02", TODAY, "en-US")).toBe("Fri, Jan 2");
    expect(dueText("2027-10-09", TODAY, "en-US")).toBe("Oct 9, 2027");
    expect(dueText("2025-12-31", TODAY, "en-US")).toBe("Dec 31, 2025");
  });

  it("counts days across a year's end", () => {
    expect(dueText("2027-01-01", "2026-12-31", "en-US")).toBe("Tomorrow");
    expect(dueText("2026-12-31", "2027-01-01", "en-US")).toBe("Yesterday");
  });
});

describe("isOverdue", () => {
  it("is an open task due before today", () => {
    expect(isOverdue({ due: "2026-10-08", completed: false }, TODAY)).toBe(true);
    expect(isOverdue({ due: "2026-10-09", completed: false }, TODAY)).toBe(false);
    expect(isOverdue({ due: "2026-10-08", completed: true }, TODAY)).toBe(false);
    expect(isOverdue({ due: "", completed: false }, TODAY)).toBe(false);
  });
});

describe("sourceTasks", () => {
  const opts = { today: TODAY, showCompleted: false, ticked: none };

  it("keeps Today's open tasks due today or earlier", () => {
    expect(ids(sourceTasks(TASKS, "today", opts))).toEqual([3, 5]);
    expect(ids(sourceTasks(TASKS, "today", { ...opts, showCompleted: true }))).toEqual([3, 5]);
  });

  it("keeps a task ticked here in its place", () => {
    expect(ids(sourceTasks(TASKS, "today", { ...opts, ticked: new Set([4]) }))).toEqual([3, 4, 5]);
    expect(ids(sourceTasks(TASKS, 10, { ...opts, ticked: new Set([4]) }))).toEqual([1, 2, 3, 4]);
  });

  it("shows every open task, or a list's, and completed ones with Show Completed", () => {
    expect(ids(sourceTasks(TASKS, "all", opts))).toEqual([1, 2, 3, 5, 6]);
    expect(ids(sourceTasks(TASKS, "all", { ...opts, showCompleted: true }))).toEqual([1, 2, 3, 4, 5, 6]);
    expect(ids(sourceTasks(TASKS, 10, opts))).toEqual([1, 2, 3]);
    expect(ids(sourceTasks(TASKS, 10, { ...opts, showCompleted: true }))).toEqual([1, 2, 3, 4]);
    expect(ids(sourceTasks(TASKS, 20, opts))).toEqual([5]);
  });

  it("has no tasks for Flagged Mail", () => {
    expect(sourceTasks(TASKS, "flagged", { ...opts, showCompleted: true })).toEqual([]);
  });
});

describe("openCount", () => {
  it("counts a source's open tasks", () => {
    expect(openCount(TASKS, "today", TODAY)).toBe(2);
    expect(openCount(TASKS, "all", TODAY)).toBe(5);
    expect(openCount(TASKS, 10, TODAY)).toBe(3);
    expect(openCount(TASKS, 21, TODAY)).toBe(1);
    expect(openCount(TASKS, 99, TODAY)).toBe(0);
    expect(openCount(TASKS, "flagged", TODAY)).toBe(0);
  });
});

describe("taskLines", () => {
  const byId = (id: number) => TASKS.find((t) => t.id === id) ?? task({ id, title: "?" });

  it("puts a subtask under its parent when the parent is shown", () => {
    expect(taskLines([1, 2, 3].map(byId))).toEqual([
      { kind: "task", task: byId(1), level: 0 },
      { kind: "task", task: byId(2), level: 1 },
      { kind: "task", task: byId(3), level: 0 },
    ]);
    expect(taskLines([2, 3].map(byId))).toEqual([
      { kind: "task", task: byId(2), level: 0 },
      { kind: "task", task: byId(3), level: 0 },
    ]);
  });

  it("heads each list's tasks with its name when names are given", () => {
    const names = new Map([
      [10, "Work"],
      [20, "Home"],
    ]);
    expect(taskLines([1, 2, 5].map(byId), names)).toEqual([
      { kind: "header", listId: 10, name: "Work" },
      { kind: "task", task: byId(1), level: 0 },
      { kind: "task", task: byId(2), level: 1 },
      { kind: "header", listId: 20, name: "Home" },
      { kind: "task", task: byId(5), level: 0 },
    ]);
  });

  it("is empty for no tasks", () => {
    expect(taskLines([], new Map())).toEqual([]);
  });
});

describe("dueSoon", () => {
  const tasks = [
    ...TASKS,
    task({ id: 7, title: "Edge", due: "2026-10-16" }),
    task({ id: 8, title: "Later", due: "2026-10-17" }),
    task({ id: 9, title: "Also yesterday", listId: 20, due: "2026-10-08" }),
  ];

  it("takes open tasks due within seven days or earlier, by due date then order", () => {
    expect(ids(dueSoon(tasks, TODAY, 10))).toEqual([3, 9, 5, 1, 7]);
  });

  it("stops at the limit", () => {
    expect(ids(dueSoon(tasks, TODAY, 2))).toEqual([3, 9]);
  });
});
