// CONTRACT TEST for task card T-0118 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { ReminderBanner } from "./MessageHeader";

describe("ReminderBanner", () => {
  it("says when, with Change… and Clear", () => {
    const onChange = vi.fn();
    const onClear = vi.fn();
    render(<ReminderBanner when="Tomorrow at 8:00 AM" onChange={onChange} onClear={onClear} />);
    const banner = screen.getByRole("status");
    expect(banner.className).toContain("bg-banner");
    expect(banner.textContent).toContain("Remind Me: Tomorrow at 8:00 AM");
    fireEvent.click(within(banner).getByRole("button", { name: "Change…" }));
    fireEvent.click(within(banner).getByRole("button", { name: "Clear" }));
    expect([onChange.mock.calls.length, onClear.mock.calls.length]).toEqual([1, 1]);
  });

  it("is read-only on a read-only account", () => {
    render(<ReminderBanner when="Today at 9:00 PM" disabled onChange={vi.fn()} onClear={vi.fn()} />);
    for (const name of ["Change…", "Clear"]) {
      expect((screen.getByRole("button", { name }) as HTMLButtonElement).disabled).toBe(true);
    }
  });
});
