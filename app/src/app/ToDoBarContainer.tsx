// Mail's small month, upcoming occurrences and shared due tasks.
import { useMemo } from "react";
import { useClient } from "../data/session";
import { useUI } from "../data/stores";
import { ToDoBar } from "../features/tasks/ToDoBar";
import { addDays, dayStart, monthGrid } from "../lib/calendarDates";
import { busyDates } from "../lib/eventLayout";
import { dueSoon } from "../lib/taskText";
import { openOccurrence, useCalendarColors, useCalendarFrame, useCalendarRange } from "./useCalendar";
import type { TasksData } from "./useTasks";

/** ToDoBarContainer connects Mail's optional pane to calendar and task data. */
export function ToDoBarContainer({ tasks }: { tasks: TasksData }) {
  const client = useClient();
  const ui = useUI();
  const frame = useCalendarFrame();
  const colors = useCalendarColors();
  const days = useMemo(() => monthGrid(frame.date, frame.weekStart), [frame.date, frame.weekStart]);
  const smallMonth = useCalendarRange(
    client,
    true,
    days[0] ?? frame.date,
    addDays(days.at(-1) ?? frame.date, 1),
    frame.timeZone,
    frame.now.getTime(),
  );
  const upcoming = useCalendarRange(
    client,
    true,
    frame.today,
    addDays(frame.today, 8),
    frame.timeZone,
    frame.now.getTime(),
  );
  const occurrences = upcoming
    .filter(
      (occurrence) =>
        (occurrence.allDay ? dayStart(occurrence.endDate, frame.timeZone) : new Date(occurrence.end)) > frame.now,
    )
    .slice(0, 5);
  const busy = useMemo(() => busyDates(smallMonth, days, frame.timeZone), [smallMonth, days, frame.timeZone]);
  return (
    <ToDoBar
      selected={frame.date}
      today={frame.today}
      weekStart={frame.weekStart}
      busy={busy}
      locale={frame.locale}
      timeZone={frame.timeZone}
      now={frame.now}
      colors={colors}
      occurrences={occurrences}
      tasks={dueSoon(tasks.tasks, frame.today, 10).map((task) => ({
        ...task,
        readOnly: task.readOnly || !!tasks.lists.find((list) => list.id === task.listId)?.readOnly,
      }))}
      messages={tasks.messages.slice(0, 5)}
      onSelect={(date) => {
        ui.setCalendarDate(date);
        ui.setModule("calendar");
      }}
      onOpen={(occurrence) => openOccurrence(occurrence, frame.timeZone)}
      onCreate={tasks.createDefault}
      onToggle={tasks.toggle}
      onClearFlag={tasks.clearFlag}
      onOpenTask={(task) => {
        ui.setModule("tasks");
        ui.setTasksSource(task.listId);
        ui.selectTask(task.id);
      }}
      onOpenMessage={tasks.openMessage}
    />
  );
}
