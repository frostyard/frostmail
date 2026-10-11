// CONTRACT TEST for task card T-0116 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { ComposeToolbar, type ComposeToolbarProps } from "./ComposeToolbar";

const TONIGHT = new Date("2026-10-12T01:00:00Z");
const TOMORROW = new Date("2026-10-12T12:00:00Z");

function toolbar(over: Partial<ComposeToolbarProps> = {}) {
  const props: ComposeToolbarProps = {
    subject: "Lunch",
    canSend: true,
    formatBarShown: false,
    maximized: false,
    laterChoices: [
      { label: "Send 9:00 PM Tonight", at: TONIGHT },
      { label: "Send 8:00 AM Tomorrow", at: TOMORROW },
    ],
    onCommand: vi.fn(),
    onSendAt: vi.fn(),
    ...over,
  };
  render(<ComposeToolbar {...props} />);
  return props;
}

const later = () => screen.getByRole("button", { name: "Send Later" }) as HTMLButtonElement;

/** entries lists a menu's rows: labels, and "—" for separators. */
function entries(menu: HTMLElement): string[] {
  return Array.from(menu.children)
    .filter((el) => el.tagName === "HR" || el.tagName === "BUTTON")
    .map((el) => (el.tagName === "HR" ? "—" : (el.querySelector(".flex-1")?.textContent ?? "")));
}

describe("ComposeToolbar's Send Later", () => {
  it("follows Send and is enabled with it", () => {
    toolbar({ canSend: false });
    const buttons = within(screen.getByRole("toolbar", { name: "Compose" })).getAllByRole("button");
    expect(buttons.slice(0, 2).map((b) => b.getAttribute("aria-label"))).toEqual(["Send", "Send Later"]);
    expect(later().disabled).toBe(true);
    expect(later().getAttribute("aria-haspopup")).toBe("menu");
  });

  it("offers the times, then Send Later…", () => {
    const props = toolbar();
    fireEvent.click(later());
    const menu = screen.getByRole("menu");
    expect(entries(menu)).toEqual(["Send 9:00 PM Tonight", "Send 8:00 AM Tomorrow", "—", "Send Later…"]);
    fireEvent.click(within(menu).getByText("Send 8:00 AM Tomorrow"));
    expect(props.onSendAt).toHaveBeenCalledWith(TOMORROW);
    expect(screen.queryByRole("menu")).toBeNull();

    fireEvent.click(later());
    fireEvent.click(within(screen.getByRole("menu")).getByText("Send Later…"));
    expect(props.onCommand).toHaveBeenCalledWith("sendLater");
  });
});
