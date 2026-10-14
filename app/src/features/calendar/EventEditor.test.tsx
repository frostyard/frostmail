// CONTRACT TEST for task card T-0148 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it } from "vitest";

import type { EventDraft } from "../../lib/eventDraft";
import { type CalendarChoice, EventEditor } from "./EventEditor";

// A weekly review on Thursdays in New York, eight times.
const review: EventDraft = {
  summary: "Design review",
  location: "Studio B",
  allDay: false,
  startDate: "2026-10-15",
  startTime: "09:00",
  endDate: "2026-10-15",
  endTime: "10:30",
  timeZone: "America/New_York",
  rrule: "FREQ=WEEKLY;BYDAY=TH;COUNT=8",
  calendarId: 201,
  alarms: [15, 60],
  transparent: false,
  description: "Bring screenshots.",
};

const calendars: CalendarChoice[] = [
  { id: 201, name: "Work", color: "#3366cc", account: "ann@example.com" },
  { id: 202, name: "Home", color: "#34c759", account: "ann@example.com" },
  { id: 401, name: "Family", color: "", account: "ann@icloud.example" },
];

function editor(start: EventDraft = review, opts: { readOnly?: boolean; invited?: boolean } = {}) {
  const seen: EventDraft[] = [];
  function Harness() {
    const [draft, setDraft] = useState(start);
    return (
      <EventEditor
        draft={draft}
        onChange={(d) => {
          seen.push(d);
          setDraft(d);
        }}
        errors={{}}
        calendars={calendars}
        zones={["Europe/Berlin", "America/New_York", "UTC"]}
        locale="en-US"
        {...opts}
      />
    );
  }
  render(<Harness />);
  return { last: () => seen.at(-1) };
}

const field = (name: string) => screen.getByLabelText(name) as HTMLInputElement | HTMLSelectElement;
const change = (name: string, value: string) => fireEvent.change(field(name), { target: { value } });

describe("EventEditor", () => {
  it("shows the draft's fields", () => {
    editor();
    expect(field("Title").value).toBe("Design review");
    expect((field("Title") as HTMLInputElement).placeholder).toBe("New Event");
    expect(field("Location").value).toBe("Studio B");
    expect((field("All-day") as HTMLInputElement).checked).toBe(false);
    expect([field("Start date").value, field("Start time").value]).toEqual(["2026-10-15", "09:00"]);
    expect([field("End date").value, field("End time").value]).toEqual(["2026-10-15", "10:30"]);
    expect(field("Time zone").value).toBe("America/New_York");
    expect(field("Repeat").value).toBe("weekly");
    expect(field("End repeat").value).toBe("count");
    expect(field("Repeat count").value).toBe("8");
    expect(field("Calendar").value).toBe("201");
    expect(
      within(field("Calendar"))
        .getAllByRole("group")
        .map((g) => g.getAttribute("label")),
    ).toEqual(["ann@example.com", "ann@icloud.example"]);
    expect([field("Alert 1").value, field("Alert 2").value]).toEqual(["15", "60"]);
    expect(field("Show as").value).toBe("busy");
    expect(field("Notes").value).toBe("Bring screenshots.");
  });

  it("moves the end with the start, and the rule's day with the date", () => {
    const { last } = editor();
    change("Start time", "10:00");
    expect(last()).toMatchObject({ startTime: "10:00", endTime: "11:30" });
    change("Start date", "2026-10-16");
    expect(last()).toMatchObject({
      startDate: "2026-10-16",
      endDate: "2026-10-16",
      rrule: "FREQ=WEEKLY;BYDAY=FR;COUNT=8",
    });
    change("End time", "12:00");
    expect(last()).toMatchObject({ startTime: "10:00", endTime: "12:00" });
  });

  it("changes the repeat, keeping its end", () => {
    const { last } = editor();
    change("Repeat", "daily");
    expect(last()?.rrule).toBe("FREQ=DAILY;COUNT=8");
    change("End repeat", "date");
    expect(last()?.rrule).toBe("FREQ=DAILY;UNTIL=20261016T035959Z");
    change("End repeat date", "2026-12-31");
    expect(last()?.rrule).toBe("FREQ=DAILY;UNTIL=20270101T045959Z");
    change("End repeat", "never");
    expect(last()?.rrule).toBe("FREQ=DAILY");
    change("Repeat", "never");
    expect(last()?.rrule).toBe("");
    expect(screen.queryByLabelText("End repeat")).toBeNull();
  });

  it("shows a rule the presets do not cover as its description", () => {
    editor({ ...review, rrule: "FREQ=MONTHLY;BYDAY=3TH" });
    const select = field("Repeat") as HTMLSelectElement;
    expect(select.value).toBe("custom");
    expect(select.selectedOptions[0]?.textContent).toBe("Every month on the third Thursday");
  });

  it("writes a custom rule from the repeat sheet", () => {
    const { last } = editor();
    change("Repeat", "edit");
    const sheet = within(screen.getByRole("dialog", { name: "Custom Repeat" }));
    fireEvent.click(sheet.getByRole("button", { name: "Monday" }));
    fireEvent.change(sheet.getByLabelText("Every"), { target: { value: "2" } });
    fireEvent.click(sheet.getByRole("button", { name: "OK" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(last()?.rrule).toBe("FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,TH;COUNT=8");

    change("Repeat", "edit");
    fireEvent.click(
      within(screen.getByRole("dialog", { name: "Custom Repeat" })).getByRole("button", { name: "Cancel" }),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(last()?.rrule).toBe("FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,TH;COUNT=8");
  });

  it("adds, changes and removes alerts", () => {
    const { last } = editor({ ...review, alarms: [15, 45] });
    expect((field("Alert 2") as HTMLSelectElement).selectedOptions[0]?.textContent).toBe("45 minutes before");
    fireEvent.click(screen.getByRole("button", { name: "Add Alert" }));
    expect(last()?.alarms).toEqual([15, 45, 0]);
    change("Alert 1", "1440");
    expect(last()?.alarms).toEqual([1440, 45, 0]);
    change("Alert 2", "none");
    expect(last()?.alarms).toEqual([1440, 0]);
  });

  it("turns all-day: no times, no zone, the all-day alerts", () => {
    const { last } = editor();
    fireEvent.click(field("All-day"));
    expect(last()?.allDay).toBe(true);
    expect(screen.queryByLabelText("Start time")).toBeNull();
    expect(screen.queryByLabelText("Time zone")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Add Alert" }));
    expect(last()?.alarms).toEqual([15, 60, -540]);
    expect(Array.from((field("Alert 3") as HTMLSelectElement).options).map((o) => o.textContent)).toEqual([
      "None",
      "On the day at 9:00 AM",
      "1 day before at 9:00 AM",
      "2 days before at 9:00 AM",
      "1 week before at 9:00 AM",
    ]);
  });

  it("disables everything when read-only, and all but the alerts for an invitation", () => {
    editor(review, { readOnly: true });
    for (const name of ["Title", "Start date", "Repeat", "Calendar", "Alert 1", "Show as", "Notes"]) {
      expect(field(name).disabled).toBe(true);
    }
    expect((screen.getByRole("button", { name: "Add Alert" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("leaves an invitation's alerts enabled", () => {
    const { last } = editor(review, { invited: true });
    for (const name of [
      "Title",
      "Location",
      "All-day",
      "Start time",
      "Time zone",
      "Repeat",
      "Calendar",
      "Show as",
      "Notes",
    ]) {
      expect(field(name).disabled).toBe(true);
    }
    expect(field("Alert 1").disabled).toBe(false);
    change("Alert 1", "30");
    expect(last()?.alarms).toEqual([30, 60]);
  });

  it("marks an invalid end", () => {
    render(
      <EventEditor
        draft={{ ...review, endTime: "08:00" }}
        onChange={() => {}}
        errors={{ end: "The end is before the start" }}
        calendars={calendars}
        zones={["UTC"]}
        locale="en-US"
      />,
    );
    expect(field("End time").getAttribute("aria-invalid")).toBe("true");
    expect(screen.getByText("The end is before the start")).toBeTruthy();
    // The event's own zone is offered even when the list lacks it.
    expect(Array.from((field("Time zone") as HTMLSelectElement).options).map((o) => o.value)).toEqual([
      "UTC",
      "America/New_York",
    ]);
  });
});
