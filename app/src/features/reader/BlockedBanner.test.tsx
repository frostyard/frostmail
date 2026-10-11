// CONTRACT TEST for task card T-0128 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { BlockedBanner } from "./MessageHeader";

describe("BlockedBanner", () => {
  it("says the sender is blocked and offers Unblock", () => {
    const onUnblock = vi.fn();
    render(<BlockedBanner onUnblock={onUnblock} />);
    const banner = screen.getByRole("status");
    expect(banner.className).toContain("bg-banner");
    expect(banner.textContent).toContain("This message is from a blocked sender.");
    fireEvent.click(within(banner).getByRole("button", { name: "Unblock" }));
    expect(onUnblock).toHaveBeenCalledTimes(1);
  });
});
