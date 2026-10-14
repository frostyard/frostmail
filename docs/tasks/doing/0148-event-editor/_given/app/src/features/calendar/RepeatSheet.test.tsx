// CONTRACT TEST for task card T-0148 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { defaultRepeat } from "../../lib/repeat";
import { RepeatSheet } from "./RepeatSheet";

// 2026-10-15 is the third Thursday of October.
const start = "2026-10-15";

describe("RepeatSheet", () => {
  it("starts the week on the locale's first day", () => {
    render(
      <RepeatSheet
        initial={defaultRepeat(start)}
        start={start}
        locale="en-US"
        onChoose={() => {}}
        onCancel={() => {}}
      />,
    );
    const days = within(screen.getByRole("group", { name: "Days" })).getAllByRole("button");
    expect(days.map((d) => d.getAttribute("aria-label"))).toEqual([
      "Sunday",
      "Monday",
      "Tuesday",
      "Wednesday",
      "Thursday",
      "Friday",
      "Saturday",
    ]);
    expect(days.map((d) => d.getAttribute("aria-pressed"))).toEqual([
      "false",
      "false",
      "false",
      "false",
      "true",
      "false",
      "false",
    ]);
  });

  it("starts on Monday where the locale does", () => {
    render(
      <RepeatSheet
        initial={defaultRepeat(start)}
        start={start}
        locale="en-GB"
        onChoose={() => {}}
        onCancel={() => {}}
      />,
    );
    const days = within(screen.getByRole("group", { name: "Days" })).getAllByRole("button");
    expect(days[0]?.getAttribute("aria-label")).toBe("Monday");
  });

  it("offers a monthly rule on the day or on the weekday", () => {
    const onChoose = vi.fn();
    render(
      <RepeatSheet
        initial={defaultRepeat(start)}
        start={start}
        locale="en-US"
        onChoose={onChoose}
        onCancel={() => {}}
      />,
    );
    fireEvent.change(screen.getByLabelText("Frequency"), { target: { value: "MONTHLY" } });
    expect(screen.queryByRole("group", { name: "Days" })).toBeNull();
    expect(screen.getByLabelText("On day 15")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("On the third Thursday"));
    fireEvent.change(screen.getByLabelText("Every"), { target: { value: "3" } });
    expect(screen.getByText(/months/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "OK" }));
    expect(onChoose).toHaveBeenCalledWith({ frequency: "MONTHLY", interval: 3, weekdays: ["TH"], byWeekday: true });
  });

  it("needs a day for a weekly rule, and cancels with Escape", () => {
    const onChoose = vi.fn();
    const onCancel = vi.fn();
    render(
      <RepeatSheet
        initial={defaultRepeat(start)}
        start={start}
        locale="en-US"
        onChoose={onChoose}
        onCancel={onCancel}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Thursday" }));
    expect((screen.getByRole("button", { name: "OK" }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.keyDown(screen.getByRole("dialog", { name: "Custom Repeat" }), { key: "Escape" });
    expect(onCancel).toHaveBeenCalled();
    expect(onChoose).not.toHaveBeenCalled();
  });
});
