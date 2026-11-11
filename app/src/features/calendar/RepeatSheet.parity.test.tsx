import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { repeatOf, ruleOf } from "../../lib/repeat";
import { RepeatSheet } from "./RepeatSheet";

it("shows and keeps the last weekday in a four-weekday month", () => {
  const initial = repeatOf("FREQ=MONTHLY;BYDAY=-1TH;WKST=SU", "2026-02-26");
  if (!initial) throw new Error("missing repeat");
  const onChoose = vi.fn();
  render(<RepeatSheet initial={initial} start="2026-02-26" locale="en-US" onChoose={onChoose} onCancel={() => {}} />);
  expect((screen.getByRole("radio", { name: "On the last Thursday" }) as HTMLInputElement).checked).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "OK" }));
  expect(onChoose).toHaveBeenCalledOnce();
  expect(ruleOf(onChoose.mock.calls[0]?.[0], "2026-02-26")).toBe("FREQ=MONTHLY;BYDAY=-1TH;WKST=SU");
});
