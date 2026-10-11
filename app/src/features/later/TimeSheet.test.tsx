// CONTRACT TEST for task card T-0115 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { TimeSheet, type TimeSheetProps } from "./TimeSheet";

const NY = "America/New_York";

function sheet(over: Partial<TimeSheetProps> = {}) {
  const props: TimeSheetProps = {
    title: "Send Later",
    initial: new Date("2026-10-12T12:00:00Z"), // 8:00 AM in New York
    now: new Date("2026-10-11T14:00:00Z"),
    timeZone: NY,
    onChoose: vi.fn(),
    onCancel: vi.fn(),
    ...over,
  };
  render(<TimeSheet {...props} />);
  return props;
}

const date = () => screen.getByLabelText("Date") as HTMLInputElement;
const time = () => screen.getByLabelText("Time") as HTMLInputElement;
const ok = () => screen.getByRole("button", { name: "OK" }) as HTMLButtonElement;

describe("TimeSheet", () => {
  it("starts at its time, in the zone", () => {
    sheet();
    expect(screen.getByRole("dialog", { name: "Send Later" }).getAttribute("aria-modal")).toBe("true");
    expect([date().type, date().value, time().type, time().value]).toEqual(["date", "2026-10-12", "time", "08:00"]);
    expect(document.activeElement).toBe(date());
    expect(ok().disabled).toBe(false);
  });

  it("gives the chosen instant", () => {
    const props = sheet();
    fireEvent.change(date(), { target: { value: "2026-10-14" } });
    fireEvent.change(time(), { target: { value: "17:30" } });
    fireEvent.click(ok());
    expect(props.onChoose).toHaveBeenCalledWith(new Date("2026-10-14T21:30:00Z"));
  });

  it("will not choose a time that is not after now", () => {
    const props = sheet();
    fireEvent.change(date(), { target: { value: "2026-10-11" } });
    fireEvent.change(time(), { target: { value: "09:00" } });
    expect(ok().disabled).toBe(true);
    fireEvent.change(time(), { target: { value: "" } });
    expect(ok().disabled).toBe(true);
    fireEvent.click(ok());
    expect(props.onChoose).not.toHaveBeenCalled();
  });

  it("cancels with Cancel or Escape", () => {
    const props = sheet({ title: "Remind Me" });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.keyDown(screen.getByRole("dialog", { name: "Remind Me" }), { key: "Escape" });
    expect(props.onCancel).toHaveBeenCalledTimes(2);
  });
});
