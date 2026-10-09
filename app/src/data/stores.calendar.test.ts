// CONTRACT TEST for task card T-0072 (docs/tasks). Do not edit.
import { beforeEach, describe, expect, it } from "vitest";

import { useUI } from "./stores";

beforeEach(() => {
  useUI.setState(useUI.getInitialState());
});

describe("calendar UI state", () => {
  it("starts on today's week with nothing selected", () => {
    const ui = useUI.getState();
    expect(ui.calendarView).toBe("week");
    expect(ui.calendarDate).toBe("");
    expect(ui.calendarSelected).toBeNull();
    expect(ui.calendarFocus).toBe("list");
  });

  it("selects an occurrence with its date, and clears it keeping the date", () => {
    useUI.getState().selectOccurrence({ eventId: 7, recurrenceId: "2026-10-09T09:00:00.000Z" }, "2026-10-09");
    expect(useUI.getState().calendarSelected).toEqual({ eventId: 7, recurrenceId: "2026-10-09T09:00:00.000Z" });
    expect(useUI.getState().calendarDate).toBe("2026-10-09");
    useUI.getState().selectOccurrence(null);
    expect(useUI.getState().calendarSelected).toBeNull();
    expect(useUI.getState().calendarDate).toBe("2026-10-09");
  });

  it("changes the view, date and focus without losing the selection", () => {
    const ui = useUI.getState();
    ui.selectOccurrence({ eventId: 7, recurrenceId: "" }, "2026-10-09");
    ui.setCalendarView("month");
    ui.setCalendarDate("2026-11-02");
    ui.setCalendarFocus("reader");
    const after = useUI.getState();
    expect([after.calendarView, after.calendarDate, after.calendarFocus]).toEqual(["month", "2026-11-02", "reader"]);
    expect(after.calendarSelected).toEqual({ eventId: 7, recurrenceId: "" });
  });
});
