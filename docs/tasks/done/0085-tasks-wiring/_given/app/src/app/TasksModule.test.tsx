// CONTRACT TEST for task card T-0085 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { today } from "../lib/calendarDates";
import { Client } from "../rpc/gen/api";
import { mockData, TASK_LISTS } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

async function setup() {
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

const key = (k: string, mods: { ctrlKey?: boolean; shiftKey?: boolean } = {}) =>
  fireEvent.keyDown(window, { key: k, ...mods });
const ctrl = (k: string) => key(k, { ctrlKey: true });
const list = (name: string) => screen.getByRole("listbox", { name });
const ids = (name: string) =>
  within(list(name))
    .queryAllByRole("option")
    .map((o) => o.getAttribute("data-id"));
const option = (name: string, id: number | string) => {
  const found = within(list(name))
    .queryAllByRole("option")
    .find((o) => o.getAttribute("data-id") === String(id));
  if (!found) throw new Error(`no row ${id} in ${name}`);
  return found;
};
const circle = (name: string, id: number) => within(option(name, id)).getByRole("checkbox", { name: "Completed" });
const sidebarRow = (name: string) => screen.getByRole("treeitem", { name: new RegExp(`^${name}`) });
const count = (name: string) => sidebarRow(name).querySelector("[data-count]")?.textContent ?? null;
const heading = () => document.querySelector("[data-segment='tasks']")?.textContent ?? "";

async function showTasks() {
  ctrl("4");
  await screen.findByRole("listbox", { name: "Today" });
  await waitFor(() => expect(ids("Today")).toEqual(["403", "404", "406"]));
}

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

describe("Tasks module", () => {
  it("opens with Ctrl+4 on Today, and from the module bar", async () => {
    await setup();
    const bar = screen.getByRole("toolbar", { name: "Modules" });
    expect(
      within(bar)
        .getAllByRole("button")
        .map((b) => b.getAttribute("aria-label")),
    ).toEqual(["Mail", "Calendar", "People", "Tasks"]);
    await showTasks();
    expect(useUI.getState().module).toBe("tasks");
    expect(screen.queryByRole("tree", { name: "Mailboxes" })).toBeNull();
    expect(
      within(screen.getByRole("toolbar", { name: "Modules" }))
        .getByRole("button", { name: "Tasks" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(heading()).toContain("Today");
    expect(screen.queryByRole("button", { name: "Show Completed" })).toBeNull();
    const headers = [...list("Today").querySelectorAll("[data-list]")].map((h) => h.textContent);
    expect(headers).toEqual(["Tasks", "Errands"]);
    expect(screen.getByText("No Task Selected")).toBeTruthy();
    ctrl("1");
    await screen.findByRole("tree", { name: "Mailboxes" });
    fireEvent.click(within(screen.getByRole("toolbar", { name: "Modules" })).getByRole("button", { name: "Tasks" }));
    await screen.findByRole("listbox", { name: "Today" });
  });

  it("lists the smart lists and the task lists with their counts", async () => {
    await setup();
    await showTasks();
    const tree = screen.getByRole("tree", { name: "Task Lists" });
    expect(
      within(tree)
        .getAllByRole("treeitem")
        .map((r) => r.getAttribute("data-key")),
    ).toEqual([
      "today",
      "all",
      "flagged",
      `list:${TASK_LISTS.tasks}`,
      `list:${TASK_LISTS.errands}`,
      `list:${TASK_LISTS.team}`,
    ]);
    expect([count("Today"), count("All Tasks"), count("Tasks"), count("Errands"), count("Team")]).toEqual([
      "3",
      "7",
      "4",
      "2",
      "1",
    ]);
    await waitFor(() => expect(Number(count("Flagged Mail"))).toBeGreaterThanOrEqual(2));
  });

  it("shows a list, its subtasks and, on request, its completed tasks", async () => {
    await setup();
    await showTasks();
    fireEvent.click(sidebarRow("Tasks"));
    await waitFor(() => expect(ids("Tasks")).toEqual(["401", "402", "403", "404"]));
    expect(option("Tasks", 402).style.paddingLeft).toBe("40px");
    expect(heading()).toContain("Tasks");
    const completed = screen.getByRole("button", { name: "Show Completed" });
    expect(completed.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(completed);
    await waitFor(() => expect(ids("Tasks")).toEqual(["401", "402", "403", "404", "405"]));
    expect(circle("Tasks", 405).getAttribute("aria-checked")).toBe("true");
    fireEvent.click(sidebarRow("All Tasks"));
    await waitFor(() => expect(ids("All Tasks")).toContain("405"));
  });

  it("ticks a task, keeping it until the source changes", async () => {
    const { calls } = await setup();
    await showTasks();
    fireEvent.click(circle("Today", 406));
    await waitFor(() => expect(calls("tasks.update")).toContainEqual({ id: 406, completed: true }));
    await waitFor(() => expect(count("Today")).toBe("2"));
    expect(ids("Today")).toEqual(["403", "404", "406"]);
    expect(circle("Today", 406).getAttribute("aria-checked")).toBe("true");
    expect(useUI.getState().tasksSelected).toBeNull();
    fireEvent.click(sidebarRow("Errands"));
    await waitFor(() => expect(ids("Errands")).toEqual(["407"]));
  });

  it("creates tasks: due today in Today, in the list chosen otherwise", async () => {
    const { calls } = await setup();
    await showTasks();
    ctrl("n");
    const field = screen.getByRole("textbox", { name: "New Task" });
    expect(document.activeElement).toBe(field);
    fireEvent.change(field, { target: { value: "Call Ann" } });
    fireEvent.keyDown(field, { key: "Enter" });
    await waitFor(() =>
      expect(calls("tasks.create")).toContainEqual({ title: "Call Ann", due: today("UTC", new Date()) }),
    );
    await waitFor(() => expect(ids("Today")).toEqual(["409", "403", "404", "406"]));
    fireEvent.click(sidebarRow("Errands"));
    await screen.findByRole("listbox", { name: "Errands" });
    fireEvent.click(screen.getByRole("button", { name: "New Task" }));
    const errands = screen.getByRole("textbox", { name: "New Task" });
    expect(document.activeElement).toBe(errands);
    fireEvent.change(errands, { target: { value: "Eggs" } });
    fireEvent.keyDown(errands, { key: "Enter" });
    await waitFor(() => expect(calls("tasks.create")).toContainEqual({ title: "Eggs", listId: TASK_LISTS.errands }));
    await waitFor(() => expect(ids("Errands")).toEqual(["410", "406", "407"]));
  });

  it("moves, ticks, deletes and opens with the keys", async () => {
    const { calls } = await setup();
    await showTasks();
    fireEvent.click(option("Today", 403));
    expect(useUI.getState().tasksSelected).toBe(403);
    expect(screen.getByRole("textbox", { name: "Title" })).toHaveProperty("value", "Renew the passport");
    key("ArrowDown");
    await waitFor(() => expect(option("Today", 404).getAttribute("aria-selected")).toBe("true"));
    key(" ");
    await waitFor(() => expect(calls("tasks.update")).toContainEqual({ id: 404, completed: true }));
    key("Delete");
    await waitFor(() => expect(calls("tasks.delete")).toContainEqual({ id: 404 }));
    await waitFor(() => expect(ids("Today")).toEqual(["403", "406"]));
    expect(useUI.getState().tasksSelected).toBe(406);
    key("Home");
    expect(useUI.getState().tasksSelected).toBe(403);
    key("End");
    expect(useUI.getState().tasksSelected).toBe(406);
    key("Enter");
    const title = screen.getByRole("textbox", { name: "Title" });
    expect(document.activeElement).toBe(title);
    expect(title).toHaveProperty("value", "Pick up the dry cleaning");
    fireEvent.click(option("Today", 403));
    key("Escape");
    await waitFor(() => expect(screen.getByText("No Task Selected")).toBeTruthy());
  });

  it("edits and deletes from the pane", async () => {
    const { calls } = await setup();
    await showTasks();
    fireEvent.click(sidebarRow("Errands"));
    await waitFor(() => expect(ids("Errands")).toEqual(["406", "407"]));
    fireEvent.click(option("Errands", 407));
    const title = screen.getByRole("textbox", { name: "Title" });
    fireEvent.change(title, { target: { value: "Buy oat milk and eggs" } });
    fireEvent.keyDown(title, { key: "Enter" });
    await waitFor(() => expect(calls("tasks.update")).toContainEqual({ id: 407, title: "Buy oat milk and eggs" }));
    await waitFor(() => expect(option("Errands", 407).textContent).toContain("Buy oat milk and eggs"));
    fireEvent.change(screen.getByLabelText("Due"), { target: { value: "2026-12-01" } });
    await waitFor(() => expect(calls("tasks.update")).toContainEqual({ id: 407, due: "2026-12-01" }));
    fireEvent.click(screen.getByRole("button", { name: "Delete Task" }));
    await waitFor(() => expect(calls("tasks.delete")).toContainEqual({ id: 407 }));
    await waitFor(() => expect(ids("Errands")).toEqual(["406"]));
  });

  it("changes nothing in a read-only list", async () => {
    await setup();
    await showTasks();
    fireEvent.click(sidebarRow("Team"));
    await waitFor(() => expect(ids("Team")).toEqual(["408"]));
    expect(screen.queryByRole("textbox", { name: "New Task" })).toBeNull();
    expect((circle("Team", 408) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(option("Team", 408));
    expect((screen.getByRole("textbox", { name: "Title" }) as HTMLInputElement).readOnly).toBe(true);
  });

  it("lists flagged mail, clears a flag, and opens a message in Mail", async () => {
    const { calls } = await setup();
    await showTasks();
    fireEvent.click(sidebarRow("Flagged Mail"));
    await screen.findByRole("listbox", { name: "Flagged Mail" });
    const row = (subject: string) =>
      within(list("Flagged Mail"))
        .getAllByRole("option")
        .find((o) => o.textContent?.includes(subject));
    await waitFor(() => expect(row("Re: Offsite plan")).toBeTruthy());
    expect(screen.queryByRole("textbox", { name: "New Task" })).toBeNull();
    const offsite = Number(row("Re: Offsite plan")?.getAttribute("data-id"));
    fireEvent.click(within(row("Re: Offsite plan") as HTMLElement).getByRole("checkbox", { name: "Completed" }));
    await waitFor(() =>
      expect(calls("message.setFlags")).toContainEqual({ ids: [offsite], changes: { flagColor: 0 } }),
    );
    await waitFor(() => expect(row("Re: Offsite plan")).toBeUndefined());
    const contract = row("Signed contract and invoice") as HTMLElement;
    const id = Number(contract.getAttribute("data-id"));
    fireEvent.click(contract);
    expect(screen.getByRole("heading", { level: 2 }).textContent).toBe("Signed contract and invoice");
    fireEvent.click(screen.getByRole("button", { name: "Open in Mail" }));
    await waitFor(() => expect(useUI.getState().module).toBe("mail"));
    await waitFor(() => expect(useUI.getState().selected).toEqual([id]));
  });

  it("opens the message a task was made from", async () => {
    await setup();
    await showTasks();
    fireEvent.click(option("Today", 404));
    fireEvent.click(screen.getByRole("button", { name: "Open Message" }));
    await waitFor(() => expect(useUI.getState().module).toBe("mail"));
    await waitFor(() => expect(useUI.getState().selected).toHaveLength(1));
  });

  it("follows changes made elsewhere", async () => {
    const { mock } = await setup();
    await showTasks();
    fireEvent.click(sidebarRow("Errands"));
    await waitFor(() => expect(ids("Errands")).toEqual(["406", "407"]));
    await new Client(mock).tasks.create({ title: "From elsewhere", listId: TASK_LISTS.errands });
    await waitFor(() => expect(ids("Errands")).toEqual(["409", "406", "407"]));
  });

  it("toggles the To-Do bar setting from Mail's toolbar", async () => {
    await setup();
    const button = screen.getByRole("button", { name: "To-Do Bar" });
    expect(button.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(button);
    await waitFor(() => expect(useUI.getState().todoBar).toBe(true));
    expect(screen.getByRole("button", { name: "To-Do Bar" }).getAttribute("aria-pressed")).toBe("true");
  });
});
