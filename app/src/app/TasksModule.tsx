// Tasks panes and keyboard actions.
import { forwardRef, useImperativeHandle, useRef } from "react";
import { type Pane, useUI } from "../data/stores";
import { ModuleBar } from "../features/sidebar/ModuleBar";
import { TaskList } from "../features/tasks/TaskList";
import { TaskPane } from "../features/tasks/TaskPane";
import { TasksSidebar } from "../features/tasks/TasksSidebar";
import type { Command } from "../lib/keymap";
import { taskLines } from "../lib/taskText";
import { Splitter } from "./Splitter";
import type { TasksData } from "./useTasks";

/** TasksHandle focuses panes and editing fields. */
export interface TasksHandle {
  focus: (pane: Pane) => void;
  focusNewTask: () => void;
  focusTitle: () => void;
}

/** tasksCommand handles task navigation and writes. */
export function tasksCommand(command: Command, data: TasksData, handle: TasksHandle | null): boolean {
  const ui = useUI.getState();
  if (command === "escape") ui.selectTask(null);
  else if (command === "compose") handle?.focusNewTask();
  else {
    if (ui.tasksFocus !== "list") return false;
    const flagged = ui.tasksSource === "flagged";
    const rows = flagged ? data.messages : data.shown;
    const current = rows.findIndex((row) => row.id === ui.tasksSelected);
    let index: number;
    switch (command) {
      case "previous":
        index = Math.max(0, current - 1);
        break;
      case "next":
        index = Math.min(rows.length - 1, current + 1);
        break;
      case "first":
        index = 0;
        break;
      case "last":
        index = rows.length - 1;
        break;
      case "pageDown": {
        const task = data.shown.find((task) => task.id === ui.tasksSelected);
        if (flagged && ui.tasksSelected !== null) data.clearFlag(ui.tasksSelected);
        else if (task) data.toggle(task.id, !task.completed);
        return true;
      }
      case "delete":
        if (!flagged && ui.tasksSelected !== null) data.remove(ui.tasksSelected);
        return true;
      case "open":
        if (flagged && ui.tasksSelected !== null) data.openMessage(ui.tasksSelected);
        else handle?.focusTitle();
        return true;
      default:
        return false;
    }
    const row = rows[index];
    if (row) ui.selectTask(row.id);
  }
  return true;
}

interface TasksModuleProps {
  data: TasksData;
  resizeSidebar: (delta: number) => void;
  endDrag: () => void;
}

/** TasksModule connects the sidebar, list and task pane. */
export const TasksModule = forwardRef<TasksHandle, TasksModuleProps>(function TasksModule(
  { data, resizeSidebar, endDrag },
  ref,
) {
  const ui = useUI();
  const sidebar = useRef<HTMLFieldSetElement>(null);
  const list = useRef<HTMLElement>(null);
  const reader = useRef<HTMLElement>(null);
  const newTask = useRef<HTMLInputElement>(null);
  const title = useRef<HTMLInputElement>(null);
  useImperativeHandle(
    ref,
    () => ({
      focus: (pane) => {
        if (pane === "sidebar") sidebar.current?.focus();
        else if (pane === "list") list.current?.focus();
        else reader.current?.focus();
      },
      focusNewTask: () => newTask.current?.focus(),
      focusTitle: () => title.current?.focus(),
    }),
    [],
  );
  const focus = (pane: Pane) => ({
    onFocus: () => ui.setTasksFocus(pane),
    onClick: () => ui.setTasksFocus(pane),
    onKeyDown: () => ui.setTasksFocus(pane),
  });
  const flagged = ui.tasksSource === "flagged";
  const task = flagged ? null : (data.shown.find((task) => task.id === ui.tasksSelected) ?? null);
  const message = flagged ? (data.messages.find((message) => message.id === ui.tasksSelected) ?? null) : null;
  const names = new Map(data.lists.map((list) => [list.id, list.name]));
  return (
    <div className="flex min-h-0 flex-1">
      {ui.sidebarVisible && (
        <>
          <fieldset
            ref={sidebar}
            aria-label="Task Lists"
            tabIndex={-1}
            {...focus("sidebar")}
            className="m-0 flex h-full shrink-0 flex-col border-0 bg-sidebar p-0 outline-none"
            style={{ width: ui.sidebarWidth }}
          >
            <div className="min-h-0 flex-1 overflow-y-auto">
              <TasksSidebar
                sections={data.sections}
                counts={data.counts}
                selected={ui.tasksSource}
                focused={ui.tasksFocus === "sidebar"}
                onSelect={ui.setTasksSource}
              />
            </div>
            <ModuleBar modules={["mail", "calendar", "people", "tasks"]} current={ui.module} onSelect={ui.setModule} />
          </fieldset>
          <Splitter label="Resize sidebar" onResize={resizeSidebar} onEnd={endDrag} />
        </>
      )}
      <section
        ref={list}
        aria-label="Task list"
        tabIndex={-1}
        {...focus("list")}
        className="h-full min-w-0 flex-1 outline-none"
      >
        <TaskList
          title={data.title}
          lines={taskLines(data.shown, typeof ui.tasksSource === "number" ? undefined : names)}
          messages={flagged ? data.messages : null}
          selected={ui.tasksSelected}
          focused={ui.tasksFocus === "list"}
          canCreate={!flagged && !data.lists.find((list) => list.id === ui.tasksSource)?.readOnly}
          today={data.today}
          now={data.now}
          locale={data.locale}
          newTaskRef={newTask}
          onSelect={ui.selectTask}
          onToggle={flagged ? data.clearFlag : data.toggle}
          onCreate={data.create}
        />
      </section>
      <section
        ref={reader}
        aria-label="Task details"
        tabIndex={-1}
        {...focus("reader")}
        className="h-full w-[320px] shrink-0 border-l border-separator outline-none"
      >
        <TaskPane
          task={task}
          message={message}
          list={data.lists.find((list) => list.id === task?.listId) ?? null}
          timeZone={data.timeZone}
          locale={data.locale}
          now={data.now}
          titleRef={title}
          onChange={data.edit}
          onDelete={data.remove}
          onOpenMessage={data.openMessage}
          onClearFlag={data.clearFlag}
        />
      </section>
    </div>
  );
});
