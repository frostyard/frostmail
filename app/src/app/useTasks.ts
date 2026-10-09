// Shared Tasks and To-Do bar data, with event refreshes and optimistic writes.
import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { useClient } from "../data/session";
import { useMail, useUI } from "../data/stores";
import { ViewModel } from "../data/view";
import type { TaskEdit } from "../features/tasks/TaskPane";
import type { TaskListSection } from "../features/tasks/TasksSidebar";
import { openCount, sourceTasks } from "../lib/taskText";
import type { Client, Collection, Event, MessageSummary, Task } from "../rpc/gen/api";
import { revealMessage } from "./openMessage";
import { useCalendarFrame } from "./useCalendar";

const noSubscribe = () => () => {};
const zero = () => 0;

function useFlagged(client: Client, active: boolean) {
  const [model, setModel] = useState<ViewModel | null>(null);
  useEffect(() => {
    if (!active) {
      setModel(null);
      return;
    }
    const view = new ViewModel(client, { flagged: true });
    setModel(view);
    view.ensure(0, 200);
    return () => view.close();
  }, [client, active]);
  useSyncExternalStore(model?.subscribe ?? noSubscribe, model?.getVersion ?? zero);
  const messages: MessageSummary[] = [];
  if (active && model) {
    for (let i = 0; i < Math.min(200, model.count); i++) {
      const row = model.row(i);
      if (row) messages.push(row);
    }
  }
  return { messages, count: active ? (model?.count ?? 0) : 0 };
}

function useTaskRecords(client: Client, active: boolean) {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [collections, setCollections] = useState<Collection[]>([]);
  const generation = useRef({ tasks: 0, collections: 0 });
  const refresh = useCallback(() => {
    const request = ++generation.current.tasks;
    void client.tasks
      .list({ completed: true })
      .then((result) => {
        if (request === generation.current.tasks) setTasks(result);
      })
      .catch((err: unknown) => console.warn("tasks list", err));
  }, [client]);
  const loadCollections = useCallback(() => {
    const request = ++generation.current.collections;
    void client.account
      .collections({ kind: "tasklist" })
      .then((result) => {
        if (request === generation.current.collections) setCollections(result.filter((list) => list.enabled));
      })
      .catch((err: unknown) => console.warn("task lists", err));
  }, [client]);
  useEffect(() => {
    if (!active) return;
    refresh();
    loadCollections();
    const timers = new Map<Event["event"], ReturnType<typeof setTimeout>>();
    const off = client.transport.onEvent((event) => {
      const load =
        event.event === "tasks.changed" ? refresh : event.event === "account.changed" ? loadCollections : null;
      if (!load) return;
      clearTimeout(timers.get(event.event));
      timers.set(event.event, setTimeout(load, 100));
    });
    return () => {
      for (const timer of timers.values()) clearTimeout(timer);
      generation.current.tasks++;
      generation.current.collections++;
      off();
    };
  }, [client, active, refresh, loadCollections]);
  return { tasks, collections, setTasks, refresh, generation };
}

/** useTasks supplies the Tasks module and Mail's optional To-Do bar. */
export function useTasks() {
  const client = useClient();
  const ui = useUI();
  const active = ui.module === "tasks" || (ui.module === "mail" && ui.todoBar);
  const frame = useCalendarFrame();
  const records = useTaskRecords(client, active);
  const { tasks, collections, setTasks, refresh, generation } = records;
  const accounts = useMail((state) => state.accounts);
  const flagged = useFlagged(client, active);
  const [ticks, setTicks] = useState({ source: ui.tasksSource, ids: new Set<number>() });
  const ticked = ticks.source === ui.tasksSource ? ticks.ids : new Set<number>();
  useEffect(() => setTicks({ source: ui.tasksSource, ids: new Set() }), [ui.tasksSource]);
  const sections: TaskListSection[] = accounts
    .map((account) => ({
      accountId: account.id,
      title: account.email,
      lists: collections
        .filter((list) => list.accountId === account.id)
        .map((list) => ({
          id: list.id,
          name: list.name,
          readOnly: list.readOnly || account.readOnly,
          count: openCount(tasks, list.id, frame.today),
        })),
    }))
    .filter((section) => section.lists.length > 0);
  const lists = sections.flatMap((section) => section.lists.map((list) => ({ ...list, account: section.title })));
  const shown = sourceTasks(tasks, ui.tasksSource, {
    today: frame.today,
    showCompleted: ui.tasksShowCompleted,
    ticked,
  });
  const rows = ui.tasksSource === "flagged" ? flagged.messages : shown;
  useEffect(() => {
    if (active && ui.tasksSelected !== null && !rows.some((row) => row.id === ui.tasksSelected)) ui.selectTask(null);
  }, [active, rows, ui.tasksSelected, ui.selectTask]);
  const writable = (id: number) => {
    const task = tasks.find((task) => task.id === id);
    return task && !task.readOnly && !lists.find((list) => list.id === task.listId)?.readOnly;
  };
  const failed = (label: string, err: unknown) => {
    console.warn(label, err);
    refresh();
  };
  const toggle = (id: number, completed: boolean) => {
    if (!writable(id)) return;
    generation.current.tasks++;
    setTasks((previous) => previous.map((task) => (task.id === id ? { ...task, completed } : task)));
    if (completed) setTicks((previous) => ({ source: ui.tasksSource, ids: new Set([...previous.ids, id]) }));
    void client.tasks.update({ id, completed }).catch((err: unknown) => failed("toggle task", err));
  };
  const create = (title: string) => {
    if (ui.tasksSource === "flagged" || lists.find((list) => list.id === ui.tasksSource)?.readOnly) return;
    const params =
      typeof ui.tasksSource === "number"
        ? { listId: ui.tasksSource }
        : ui.tasksSource === "today"
          ? { due: frame.today }
          : {};
    void client.tasks.create({ title, ...params }).catch((err: unknown) => console.warn("create task", err));
  };
  const createDefault = (title: string) => {
    void client.tasks.create({ title }).catch((err: unknown) => console.warn("create task", err));
  };
  const edit = (id: number, change: TaskEdit) => {
    if (!writable(id)) return;
    void client.tasks.update({ id, ...change }).catch((err: unknown) => console.warn("edit task", err));
  };
  const remove = (id: number) => {
    if (!writable(id)) return;
    const removed = new Set([id, ...tasks.filter((task) => task.parentId === id).map((task) => task.id)]);
    const index = shown.findIndex((task) => task.id === id);
    const remaining = shown.filter((task) => !removed.has(task.id));
    ui.selectTask(remaining[index]?.id ?? remaining[index - 1]?.id ?? null);
    generation.current.tasks++;
    setTasks((previous) => previous.filter((task) => !removed.has(task.id)));
    void client.tasks.delete({ id }).catch((err: unknown) => failed("delete task", err));
  };
  const clearFlag = (id: number) => {
    void client.message
      .setFlags({ ids: [id], changes: { flagColor: 0 } })
      .catch((err: unknown) => console.warn("clear flag", err));
  };
  const openMessage = (id: number) => {
    void revealMessage(client, id)
      .then(() => useUI.getState().setModule("mail"))
      .catch((err: unknown) => console.warn("open message", err));
  };
  const title =
    ui.tasksSource === "today"
      ? "Today"
      : ui.tasksSource === "all"
        ? "All Tasks"
        : ui.tasksSource === "flagged"
          ? "Flagged Mail"
          : (lists.find((list) => list.id === ui.tasksSource)?.name ?? "");
  return {
    ...frame,
    tasks,
    shown,
    messages: flagged.messages,
    ticked,
    sections,
    lists,
    title,
    counts: {
      today: openCount(tasks, "today", frame.today),
      all: openCount(tasks, "all", frame.today),
      flagged: flagged.count,
    },
    toggle,
    create,
    createDefault,
    edit,
    remove,
    clearFlag,
    openMessage,
  };
}

/** TasksData is the shared Tasks data and actions. */
export type TasksData = ReturnType<typeof useTasks>;
