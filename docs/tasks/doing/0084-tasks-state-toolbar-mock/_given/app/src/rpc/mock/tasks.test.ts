// CONTRACT TEST for task card T-0084 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { Client, ErrorCode, type Event } from "../gen/api";
import { RPCError } from "../transport";
import { FIXTURE, mockData, TASK_LISTS } from "./fixture";
import { MockTransport } from "./mock";

const NOW = new Date("2026-10-09T12:00:00Z");

function client() {
  const mock = new MockTransport(mockData({ inbox: 5, now: NOW }));
  const events: Event[] = [];
  mock.onEvent((e) => events.push(e));
  return { mock, c: new Client(mock), events };
}

const titles = (tasks: { title: string }[]) => tasks.map((t) => t.title);

async function code(p: Promise<unknown>): Promise<number | undefined> {
  try {
    await p;
    return undefined;
  } catch (err) {
    return err instanceof RPCError ? err.code : -1;
  }
}

const OPEN = [
  "Send the Q3 report",
  "Draft the charts",
  "Renew the passport",
  "Reply about the offsite",
  "Pick up the dry cleaning",
  "Buy oat milk",
  "Quarterly planning",
];

describe("MockTransport tasks", () => {
  it("lists the fixture's task lists", async () => {
    const { c } = client();
    const lists = await c.account.collections({ kind: "tasklist" });
    expect(lists.map((l) => [l.id, l.name, l.readOnly, l.isDefault, l.accountId])).toEqual([
      [TASK_LISTS.tasks, "Tasks", false, true, FIXTURE.accountId],
      [TASK_LISTS.errands, "Errands", false, false, FIXTURE.accountId],
      [TASK_LISTS.team, "Team", true, false, FIXTURE.accountId],
    ]);
  });

  it("lists open tasks list by list, subtasks after their parents", async () => {
    const { c } = client();
    const all = await c.tasks.list({});
    expect(titles(all)).toEqual(OPEN);
    const [report, charts, passport, reply] = all;
    expect([report?.listId, report?.due, report?.notes.split("\n")[0]]).toEqual([
      TASK_LISTS.tasks,
      "2026-10-10",
      "Numbers from Maria",
    ]);
    expect(charts?.parentId).toBe(report?.id);
    expect(passport?.due).toBe("2026-10-07");
    expect(reply?.due).toBe("2026-10-09");
    const [linked] = await c.message.summaries({ ids: [reply?.messageId ?? -1] });
    expect(linked?.subject).toBe("Re: Offsite plan");
    expect(all.at(-1)?.readOnly).toBe(true);
    expect(all.every((t) => t.accountId === FIXTURE.accountId)).toBe(true);
  });

  it("includes completed tasks when asked", async () => {
    const { c } = client();
    const all = await c.tasks.list({ completed: true });
    expect(titles(all)).toEqual([...OPEN.slice(0, 4), "Book flights", ...OPEN.slice(4)]);
    const flights = all[4];
    expect(flights?.completed).toBe(true);
    expect(flights?.completedAt).toBe("2026-10-08T10:00:00.000Z");
  });

  it("filters by list and by due date", async () => {
    const { c } = client();
    expect(titles(await c.tasks.list({ listId: TASK_LISTS.errands }))).toEqual([
      "Pick up the dry cleaning",
      "Buy oat milk",
    ]);
    expect(titles(await c.tasks.list({ dueBefore: "2026-10-10" }))).toEqual([
      "Renew the passport",
      "Reply about the offsite",
      "Pick up the dry cleaning",
    ]);
    expect(await code(c.tasks.list({ listId: 99999 }))).toBe(ErrorCode.notFound);
    expect(await code(c.tasks.list({ dueBefore: "soon" }))).toBe(ErrorCode.invalidParams);
  });

  it("creates a task at the top of the default list, or under its parent", async () => {
    const { c, events } = client();
    const made = await c.tasks.create({ title: "  Call Ann ", due: "2026-10-12" });
    expect([made.title, made.due, made.listId, made.completed, made.readOnly]).toEqual([
      "Call Ann",
      "2026-10-12",
      TASK_LISTS.tasks,
      false,
      false,
    ]);
    expect(events.at(-1)).toEqual({ event: "tasks.changed", data: { accountId: FIXTURE.accountId } });
    const all = await c.tasks.list({});
    expect(titles(all).slice(0, 2)).toEqual(["Call Ann", "Send the Q3 report"]);
    const report = all[1];
    const sub = await c.tasks.create({ title: "Proofread", parentId: report?.id ?? -1 });
    expect(sub.parentId).toBe(report?.id);
    expect(titles(await c.tasks.list({ listId: TASK_LISTS.tasks })).slice(0, 4)).toEqual([
      "Call Ann",
      "Send the Q3 report",
      "Proofread",
      "Draft the charts",
    ]);
    const eggs = await c.tasks.create({ title: "Eggs", listId: TASK_LISTS.errands });
    expect(eggs.listId).toBe(TASK_LISTS.errands);
    expect(titles(await c.tasks.list({ listId: TASK_LISTS.errands }))[0]).toBe("Eggs");
  });

  it("refuses bad new tasks", async () => {
    const { c } = client();
    const all = await c.tasks.list({});
    const charts = all[1];
    expect(await code(c.tasks.create({ title: "  " }))).toBe(ErrorCode.invalidParams);
    expect(await code(c.tasks.create({ title: "x", due: "tomorrow" }))).toBe(ErrorCode.invalidParams);
    expect(await code(c.tasks.create({ title: "x", parentId: charts?.id ?? -1 }))).toBe(ErrorCode.invalidParams);
    expect(await code(c.tasks.create({ title: "x", listId: TASK_LISTS.team }))).toBe(ErrorCode.conflict);
    expect(await code(c.tasks.create({ title: "x", listId: 99999 }))).toBe(ErrorCode.notFound);
  });

  it("completes, reopens and edits a task", async () => {
    const { c, events } = client();
    const passport = (await c.tasks.list({}))[2];
    const id = passport?.id ?? -1;
    const done = await c.tasks.update({ id, completed: true });
    expect(done.completed).toBe(true);
    expect(Date.parse(done.completedAt ?? "")).toBe(NOW.getTime());
    expect(events.at(-1)?.event).toBe("tasks.changed");
    expect(titles(await c.tasks.list({}))).not.toContain("Renew the passport");
    const open = await c.tasks.update({ id, completed: false, title: "Renew both passports", due: "" });
    expect([open.completed, open.completedAt, open.title, open.due]).toEqual([
      false,
      undefined,
      "Renew both passports",
      "",
    ]);
    const noted = await c.tasks.update({ id, notes: "Photos first" });
    expect([noted.title, noted.notes]).toEqual(["Renew both passports", "Photos first"]);
    expect(await code(c.tasks.update({ id, title: " " }))).toBe(ErrorCode.invalidParams);
    expect(await code(c.tasks.update({ id, due: "13/10" }))).toBe(ErrorCode.invalidParams);
    expect(await code(c.tasks.update({ id: 99999, completed: true }))).toBe(ErrorCode.notFound);
  });

  it("deletes a task with its subtasks", async () => {
    const { c, events } = client();
    const report = (await c.tasks.list({}))[0];
    await c.tasks.delete({ id: report?.id ?? -1 });
    expect(events.at(-1)?.event).toBe("tasks.changed");
    expect(titles(await c.tasks.list({}))).toEqual(OPEN.slice(2));
    expect(await code(c.tasks.delete({ id: report?.id ?? -1 }))).toBe(ErrorCode.notFound);
  });

  it("changes nothing in a read-only list", async () => {
    const { c } = client();
    const planning = (await c.tasks.list({})).at(-1);
    const id = planning?.id ?? -1;
    expect(await code(c.tasks.update({ id, completed: true }))).toBe(ErrorCode.conflict);
    expect(await code(c.tasks.delete({ id }))).toBe(ErrorCode.conflict);
    expect(titles(await c.tasks.list({}))).toEqual(OPEN);
  });

  it("leaves out the tasks of a hidden list", async () => {
    const { c, events } = client();
    await c.account.setCollection({ id: TASK_LISTS.errands, enabled: false });
    expect(events).toContainEqual({ event: "tasks.changed", data: { accountId: FIXTURE.accountId } });
    expect(titles(await c.tasks.list({}))).toEqual([...OPEN.slice(0, 4), "Quarterly planning"]);
  });
});
