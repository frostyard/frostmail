// CONTRACT TEST for task card T-0128 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { ContactCard } from "../../rpc/gen/api";
import { ContactPopover, type ContactPopoverProps } from "./ContactPopover";

const card: ContactCard = {
  email: "ada@example.com",
  name: "Ada Lovelace",
  recent: [],
  upcoming: [],
  canAdd: false,
};

function popover(over: Partial<ContactPopoverProps> = {}) {
  const props: ContactPopoverProps = {
    address: { name: "Ada L.", address: "ada@example.com" },
    card,
    at: { x: 120, y: 80 },
    now: new Date(2026, 9, 8, 15, 0),
    add: "idle",
    onCompose: vi.fn(),
    onAdd: vi.fn(),
    onOpenPerson: vi.fn(),
    onOpenMessage: vi.fn(),
    onClose: vi.fn(),
    ...over,
  };
  render(<ContactPopover {...props} />);
  return props;
}

describe("the contact card's block button", () => {
  it("blocks a sender who is not blocked, after the VIP button", () => {
    const onBlock = vi.fn();
    popover({ vip: false, onVip: vi.fn(), blocked: false, onBlock });
    const names = screen.getAllByRole("button").map((b) => b.textContent);
    expect(names.indexOf("Block Contact")).toBe(names.indexOf("Add to VIPs") + 1);
    fireEvent.click(screen.getByRole("button", { name: "Block Contact" }));
    expect(onBlock).toHaveBeenCalledTimes(1);
  });

  it("unblocks a blocked sender", () => {
    const onBlock = vi.fn();
    popover({ blocked: true, onBlock });
    fireEvent.click(screen.getByRole("button", { name: "Unblock Contact" }));
    expect(onBlock).toHaveBeenCalledTimes(1);
  });

  it("is disabled while busy, and absent without onBlock", () => {
    popover({ blocked: false, blockBusy: true, onBlock: vi.fn() });
    expect((screen.getByRole("button", { name: "Block Contact" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("is absent without onBlock", () => {
    popover();
    expect(screen.queryByRole("button", { name: /Block Contact/ })).toBeNull();
  });
});
