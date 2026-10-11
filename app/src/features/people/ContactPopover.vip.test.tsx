// CONTRACT TEST for task card T-0105 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { ContactCard } from "../../rpc/gen/api";
import { ContactPopover, type ContactPopoverProps } from "./ContactPopover";

const card: ContactCard = {
  email: "ada@example.com",
  name: "Ada Lovelace",
  person: { id: 7, displayName: "Ada Lovelace", organization: "", hasPhoto: false, contacts: [] },
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

describe("the contact card's VIP button", () => {
  it("adds a sender who is not a VIP", () => {
    const onVip = vi.fn();
    popover({ vip: false, onVip });
    fireEvent.click(screen.getByRole("button", { name: "Add to VIPs" }));
    expect(onVip).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("button", { name: "Remove from VIPs" })).toBeNull();
  });

  it("removes a VIP", () => {
    const onVip = vi.fn();
    popover({ vip: true, onVip });
    fireEvent.click(screen.getByRole("button", { name: "Remove from VIPs" }));
    expect(onVip).toHaveBeenCalledTimes(1);
  });

  it("waits while busy", () => {
    popover({ vip: false, vipBusy: true, onVip: vi.fn() });
    expect((screen.getByRole("button", { name: "Add to VIPs" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("is absent without onVip", () => {
    popover({ vip: false });
    expect(screen.queryByRole("button", { name: /VIPs/ })).toBeNull();
  });
});
