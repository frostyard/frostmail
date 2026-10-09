// The tasks domain of MockTransport: the fixture's task lists and tasks, as
// maild answers tasks.* (docs/design/pim.md, Tasks; internal/engine/tasks.go).
import { type Collection, ErrorCode, type Event, type Task } from "../gen/api";
import { RPCError } from "../transport";
import { NOT_HANDLED } from "./compose";

/** MockTasksData is the tasks the mock serves, each list's in its order
 *  (a parent before its subtasks); their lists are `tasklist` collections
 *  in MockPeopleData. */
export interface MockTasksData {
  tasks: Task[];
  /** The time a task completed here is stamped with. */
  now: string;
}

type Params = Record<string, unknown>;

function validDate(value: unknown): value is string {
  if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const date = new Date(value);
  return Number.isFinite(date.getTime()) && date.toISOString().slice(0, 10) === value;
}

function title(value: unknown): string {
  if (typeof value !== "string" || value.trim() === "") {
    throw new RPCError(ErrorCode.invalidParams, "a task needs a title");
  }
  return value.trim();
}

/** MockTasks answers tasks.list, create, update and delete over the shared
 *  collections. */
export class MockTasks {
  private nextId: number;

  constructor(
    readonly data: MockTasksData,
    readonly collections: Collection[],
    readonly emit: (e: Event) => void,
  ) {
    this.nextId = Math.max(0, ...data.tasks.map((t) => t.id)) + 1;
  }

  /** dispatch answers a tasks method, or returns NOT_HANDLED. */
  dispatch(method: string, params: Params): unknown {
    switch (method) {
      case "tasks.list":
        return this.list(params);
      case "tasks.create":
        return this.create(params);
      case "tasks.update":
        return this.update(params);
      case "tasks.delete":
        return this.delete(Number(params.id));
      default:
        return NOT_HANDLED;
    }
  }

  private collection(id: number, writable = false): Collection {
    const list = this.collections.find((c) => c.id === id && c.kind === "tasklist");
    if (!list) throw new RPCError(ErrorCode.notFound, `task list ${id} does not exist`);
    if (writable && list.readOnly) throw new RPCError(ErrorCode.conflict, "the task list is read-only");
    return list;
  }

  private task(id: number): Task {
    const task = this.data.tasks.find((t) => t.id === id);
    if (!task) throw new RPCError(ErrorCode.notFound, `task ${id} does not exist`);
    return task;
  }

  private list(p: Params): Task[] {
    const lists =
      p.listId === undefined
        ? this.collections.filter((c) => c.kind === "tasklist" && c.enabled)
        : [this.collection(Number(p.listId))];
    const before = p.dueBefore;
    if (before !== undefined && !validDate(before)) {
      throw new RPCError(ErrorCode.invalidParams, "dueBefore is YYYY-MM-DD");
    }
    return lists.flatMap((list) =>
      this.data.tasks
        .filter((t) => t.listId === list.id && (!t.completed || p.completed === true))
        .filter((t) => before === undefined || (t.due !== "" && t.due < before))
        .map((t) => ({ ...t, readOnly: list.readOnly })),
    );
  }

  private create(p: Params): Task {
    const name = title(p.title);
    if (p.due !== undefined && !validDate(p.due)) {
      throw new RPCError(ErrorCode.invalidParams, "due is YYYY-MM-DD");
    }
    const lists = this.collections.filter((c) => c.kind === "tasklist");
    const listId = p.listId ?? (lists.find((c) => c.isDefault) ?? lists[0])?.id;
    const list = this.collection(Number(listId), true);
    const parent = p.parentId === undefined ? undefined : this.task(Number(p.parentId));
    if (parent && (parent.parentId !== undefined || parent.listId !== list.id)) {
      throw new RPCError(ErrorCode.invalidParams, "a subtask's parent is a top-level task of the same list");
    }
    const task: Task = {
      id: this.nextId++,
      listId: list.id,
      accountId: list.accountId,
      title: name,
      notes: typeof p.notes === "string" ? p.notes : "",
      due: typeof p.due === "string" ? p.due : "",
      completed: false,
      readOnly: list.readOnly,
      ...(parent ? { parentId: parent.id } : {}),
    };
    const index = parent ? this.data.tasks.indexOf(parent) + 1 : this.data.tasks.findIndex((t) => t.listId === list.id);
    this.data.tasks.splice(index < 0 ? this.data.tasks.length : index, 0, task);
    this.changed(list);
    return { ...task };
  }

  private update(p: Params): Task {
    const task = this.task(Number(p.id));
    const list = this.collection(task.listId, true);
    const name = p.title === undefined ? undefined : title(p.title);
    if (p.due !== undefined && p.due !== "" && !validDate(p.due)) {
      throw new RPCError(ErrorCode.invalidParams, "due is YYYY-MM-DD, or empty to clear it");
    }
    if (name !== undefined) task.title = name;
    if (typeof p.notes === "string") task.notes = p.notes;
    if (typeof p.due === "string") task.due = p.due;
    if (typeof p.completed === "boolean") {
      task.completed = p.completed;
      if (p.completed) task.completedAt = this.data.now;
      else delete task.completedAt;
    }
    this.changed(list);
    return { ...task, readOnly: list.readOnly };
  }

  private delete(id: number): null {
    const task = this.task(id);
    const list = this.collection(task.listId, true);
    this.data.tasks = this.data.tasks.filter((t) => t.id !== id && t.parentId !== id);
    this.changed(list);
    return null;
  }

  private changed(list: Collection): void {
    this.emit({ event: "tasks.changed", data: { accountId: list.accountId } });
  }
}
