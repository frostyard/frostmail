// CONTRACT TEST for task card T-0149 (docs/tasks). Do not edit.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { Client } from "../rpc/gen/api";
import { CALENDARS, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { EventWindow } from "./EventWindow";
import type { EditorTarget } from "./openEvent";

// The fixture's day is 2026-10-08 (UTC): 302 Design review, the user's;
// 303 Lunch with Ann, an invitation; 304 Holiday, read-only. A test that
// needs a series the user organizes adds Team sync, daily at 10:00 from
// October 6, five times (event 308).
async function setup(target: EditorTarget, withSeries = false) {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
  if (withSeries) {
    await new Client(mock).calendar.create({
      fields: {
        summary: "Team sync",
        allDay: false,
        start: "2026-10-06T10:00:00.000Z",
        end: "2026-10-06T10:30:00.000Z",
        timeZone: "UTC",
        rrule: "FREQ=DAILY;COUNT=5",
      },
    });
  }
  const close = vi.spyOn(window, "close").mockImplementation(() => {});
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <EventWindow target={target} />
    </Session>,
  );
  await screen.findByLabelText("Title");
  const calls = (method: string) => mock.calls.filter((c) => c.method === method);
  return { calls, close };
}

const field = (name: string) => screen.getByLabelText(name) as HTMLInputElement;
const click = (name: string) => fireEvent.click(screen.getByRole("button", { name }));

afterEach(() => {
  vi.restoreAllMocks();
});

describe("EventWindow", () => {
  it("saves only what changed, and closes", async () => {
    const { calls, close } = await setup({ kind: "edit", id: 302, recurrenceId: "" });
    expect(field("Title").value).toBe("Design review");
    expect(document.title).toBe("Design review");
    fireEvent.change(field("Title"), { target: { value: "Design review v2" } });
    expect(document.title).toBe("Design review v2");
    click("Save");
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(calls("calendar.update").at(-1)?.params).toEqual({ id: 302, fields: { summary: "Design review v2" } });
  });

  it("creates a new event in the default calendar", async () => {
    const { calls, close } = await setup({
      kind: "new",
      allDay: false,
      start: "2026-10-09T14:00:00.000Z",
      end: "2026-10-09T15:00:00.000Z",
      startDate: "",
      endDate: "",
    });
    expect(document.title).toBe("New Event");
    expect(field("Calendar").value).toBe(String(CALENDARS.work));
    fireEvent.change(field("Title"), { target: { value: "Planning" } });
    fireEvent.submit(field("Title").closest("form") as HTMLFormElement);
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(calls("calendar.create").at(-1)?.params).toEqual({
      calendarId: CALENDARS.work,
      fields: {
        summary: "Planning",
        allDay: false,
        start: "2026-10-09T14:00:00.000Z",
        end: "2026-10-09T15:00:00.000Z",
        timeZone: "UTC",
      },
    });
  });

  it("asks which occurrences a change to a series takes", async () => {
    const rid = "2026-10-08T10:00:00.000Z";
    const { calls, close } = await setup({ kind: "edit", id: 308, recurrenceId: rid }, true);
    fireEvent.change(field("Location"), { target: { value: "Room 9" } });
    click("Save");
    const sheet = within(await screen.findByRole("dialog", { name: "Change Repeating Event" }));
    fireEvent.click(sheet.getByRole("button", { name: "All Events" }));
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(calls("calendar.update").at(-1)?.params).toEqual({
      id: 308,
      recurrenceId: rid,
      scope: "all",
      fields: { location: "Room 9" },
    });
  });

  it("deletes from its footer, through the scope sheet for a series", async () => {
    const single = await setup({ kind: "edit", id: 302, recurrenceId: "" });
    click("Delete Event");
    await waitFor(() => expect(single.close).toHaveBeenCalled());
    expect(single.calls("calendar.delete").at(-1)?.params).toEqual({ id: 302 });
  });

  it("deletes one occurrence of a series", async () => {
    const rid = "2026-10-06T10:00:00.000Z";
    const { calls, close } = await setup({ kind: "edit", id: 308, recurrenceId: rid }, true);
    click("Delete Event");
    const sheet = within(await screen.findByRole("dialog", { name: "Delete Repeating Event" }));
    // The first occurrence: no All Future Events.
    expect(sheet.queryByRole("button", { name: "All Future Events" })).toBeNull();
    fireEvent.click(sheet.getByRole("button", { name: "This Event" }));
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(calls("calendar.delete").at(-1)?.params).toEqual({ id: 308, recurrenceId: rid, scope: "this" });
  });

  it("asks before closing with changes", async () => {
    const { calls, close } = await setup({ kind: "edit", id: 302, recurrenceId: "" });
    click("Cancel");
    expect(close).toHaveBeenCalledTimes(1); // nothing changed
    fireEvent.change(field("Title"), { target: { value: "Review" } });
    click("Cancel");
    const sheet = within(screen.getByRole("dialog", { name: 'Save changes to "Review"?' }));
    fireEvent.click(sheet.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(close).toHaveBeenCalledTimes(1);
    fireEvent.keyDown(field("Title"), { key: "Escape" });
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Don't Save" }));
    expect(close).toHaveBeenCalledTimes(2);
    expect(calls("calendar.update")).toHaveLength(0);
  });

  it("shows a read-only event with only Close", async () => {
    const { close } = await setup({ kind: "edit", id: 304, recurrenceId: "" });
    expect(screen.getByText("Read-only")).toBeTruthy();
    expect(field("Title").disabled).toBe(true);
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Delete Event" })).toBeNull();
    const footer = screen.getAllByRole("button", { name: "Close" });
    fireEvent.click(footer[footer.length - 1] as HTMLElement);
    expect(close).toHaveBeenCalled();
  });

  it("changes only an invitation's alerts", async () => {
    const { calls, close } = await setup({ kind: "edit", id: 303, recurrenceId: "" });
    expect(field("Title").disabled).toBe(true);
    expect(screen.queryByRole("button", { name: "Delete Event" })).toBeNull();
    click("Add Alert");
    click("Save");
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(calls("calendar.update").at(-1)?.params).toEqual({ id: 303, fields: { alarms: [0] } });
  });

  it("shows maild's refusal and stays open", async () => {
    const { close } = await setup({ kind: "edit", id: 302, recurrenceId: "" });
    fireEvent.change(field("Repeat"), { target: { value: "monthly" } });
    click("Save");
    // The mock repeats only daily and weekly, and says so.
    expect((await screen.findByRole("alert")).textContent).not.toBe("");
    expect(close).not.toHaveBeenCalled();
  });
});
