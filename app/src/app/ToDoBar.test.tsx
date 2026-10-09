// CONTRACT TEST for task card T-0086 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { addDays, today } from "../lib/calendarDates";
import { mockData, TASK_LISTS } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

async function setup(todoBar = true) {
  useUI.setState({ ...useUI.getInitialState(), todoBar });
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date() }));
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("tree", { name: "Mailboxes" });
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  return { mock, calls };
}

const bar = () => screen.getByRole("complementary", { name: "To-Do Bar" });
const items = () => within(within(bar()).getByRole("list", { name: "Due" })).getAllByRole("listitem");
const kinds = () => items().map((i) => `${i.getAttribute("data-kind")}:${i.getAttribute("data-id")}`);
const item = (kind: string, id: number | string) => {
  const found = items().find((i) => i.getAttribute("data-kind") === kind && i.getAttribute("data-id") === String(id));
  if (!found) throw new Error(`no ${kind} ${id}`);
  return found;
};
const fullDate = (date: string) =>
  new Intl.DateTimeFormat("en-US", {
    weekday: "long",
    month: "long",
    day: "numeric",
    year: "numeric",
    timeZone: "UTC",
  }).format(new Date(`${date}T00:00:00Z`));

async function due() {
  await waitFor(() =>
    expect(kinds().slice(0, 5)).toEqual(["task:403", "task:404", "task:406", "task:401", "task:408"]),
  );
}

beforeEach(() => {
  localStorage.clear();
});

describe("To-Do bar", () => {
  it("shows beside Mail's reader while on, and the toolbar button hides it", async () => {
    await setup();
    expect(within(bar()).getByRole("grid")).toBeTruthy();
    const toggle = screen.getByRole("button", { name: "To-Do Bar" });
    expect(toggle.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(toggle);
    await waitFor(() => expect(screen.queryByRole("complementary", { name: "To-Do Bar" })).toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "To-Do Bar" }));
    await screen.findByRole("complementary", { name: "To-Do Bar" });
  });

  it("is not there while off, nor outside Mail", async () => {
    await setup(false);
    expect(screen.queryByRole("complementary", { name: "To-Do Bar" })).toBeNull();
    useUI.getState().toggleTodoBar();
    await screen.findByRole("complementary", { name: "To-Do Bar" });
    fireEvent.keyDown(window, { key: "4", ctrlKey: true });
    await screen.findByRole("listbox", { name: "Today" });
    expect(screen.queryByRole("complementary", { name: "To-Do Bar" })).toBeNull();
  });

  it("lists the tasks due within a week by date, then flagged mail", async () => {
    await setup();
    await due();
    const messages = kinds().filter((k) => k.startsWith("message:"));
    expect(messages.length).toBeGreaterThanOrEqual(2);
    expect(messages.length).toBeLessThanOrEqual(5);
    expect(
      kinds()
        .slice(5)
        .every((k) => k.startsWith("message:")),
    ).toBe(true);
    expect(within(item("task", 403)).getByRole("button", { name: "Renew the passport" })).toBeTruthy();
    expect(within(item("task", 404)).getByText("Today")).toBeTruthy();
    const overdue = within(item("task", 403)).getByText((_, el) => el?.className.includes("text-flag-1") ?? false);
    expect(overdue).toBeTruthy();
  });

  it("ticks a task and clears a flag", async () => {
    const { calls } = await setup();
    await due();
    fireEvent.click(within(item("task", 403)).getByRole("checkbox", { name: "Completed" }));
    await waitFor(() => expect(calls("tasks.update")).toContainEqual({ id: 403, completed: true }));
    const message = items().find((i) => i.getAttribute("data-kind") === "message") as HTMLElement;
    const id = Number(message.getAttribute("data-id"));
    fireEvent.click(within(message).getByRole("checkbox", { name: "Completed" }));
    await waitFor(() => expect(calls("message.setFlags")).toContainEqual({ ids: [id], changes: { flagColor: 0 } }));
  });

  it("adds a task to the default list", async () => {
    const { calls } = await setup();
    await due();
    const field = within(bar()).getByRole("textbox", { name: "New Task" });
    fireEvent.change(field, { target: { value: "  Call Ann " } });
    fireEvent.keyDown(field, { key: "Enter" });
    await waitFor(() => expect(calls("tasks.create")).toContainEqual({ title: "Call Ann" }));
    expect((field as HTMLInputElement).value).toBe("");
  });

  it("opens a task in Tasks and a message in Mail", async () => {
    await setup();
    await due();
    const message = items().find((i) => i.getAttribute("data-kind") === "message") as HTMLElement;
    const id = Number(message.getAttribute("data-id"));
    const subject = within(message)
      .getAllByRole("button")
      .find((b) => b.getAttribute("role") !== "checkbox");
    fireEvent.click(subject as HTMLElement);
    await waitFor(() => expect(useUI.getState().selected).toEqual([id]));
    expect(useUI.getState().module).toBe("mail");
    fireEvent.click(within(item("task", 403)).getByRole("button", { name: "Renew the passport" }));
    await waitFor(() => expect(useUI.getState().module).toBe("tasks"));
    expect(useUI.getState().tasksSource).toBe(TASK_LISTS.tasks);
    expect(useUI.getState().tasksSelected).toBe(403);
  });

  it("opens a day and an upcoming event in Calendar", async () => {
    await setup();
    await due();
    const tomorrow = addDays(today("UTC", new Date()), 1);
    fireEvent.click(within(bar()).getByRole("button", { name: fullDate(tomorrow) }));
    await waitFor(() => expect(useUI.getState().module).toBe("calendar"));
    expect(useUI.getState().calendarDate).toBe(tomorrow);
    fireEvent.keyDown(window, { key: "1", ctrlKey: true });
    await screen.findByRole("complementary", { name: "To-Do Bar" });
    const upcoming = await within(bar()).findByRole("region", { name: "Upcoming" });
    const events = within(upcoming).getAllByRole("button");
    expect(events.length).toBeGreaterThanOrEqual(1);
    expect(events.length).toBeLessThanOrEqual(5);
    fireEvent.click(events[0] as HTMLElement);
    await waitFor(() => expect(useUI.getState().module).toBe("calendar"));
    expect(useUI.getState().calendarSelected).not.toBeNull();
  });
});
