// CONTRACT TEST for task card T-0065 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { formatListDate } from "../../lib/format";
import type { ContactCard, MessageSummary, Person } from "../../rpc/gen/api";
import { ContactPopover, type ContactPopoverProps } from "./ContactPopover";

const now = new Date(2026, 9, 8, 15, 0);

const ada: Person = {
  id: 7,
  displayName: "Ada Lovelace",
  organization: "Analytical Engines",
  hasPhoto: false,
  contacts: [],
};

function msg(id: number, subject: string, date: Date): MessageSummary {
  return {
    id,
    accountId: 1,
    mailboxIds: [1],
    threadId: id,
    subject,
    from: { name: "Ada", address: "ada@example.com" },
    date: date.toISOString(),
    preview: "",
    flags: { seen: true, flagged: false, answered: false, forwarded: false, draft: false, flagColor: 0 },
    hasAttachments: false,
    size: 1,
    threadCount: 1,
  };
}

function card(over: Partial<ContactCard> = {}): ContactCard {
  return {
    email: "ada@example.com",
    name: "Ada Lovelace",
    person: ada,
    recent: [],
    upcoming: [],
    canAdd: false,
    ...over,
  };
}

function popover(over: Partial<ContactPopoverProps> = {}) {
  const props: ContactPopoverProps = {
    address: { name: "Ada L.", address: "ada@example.com" },
    card: card(),
    at: { x: 120, y: 80 },
    now,
    add: "idle",
    onCompose: vi.fn(),
    onAdd: vi.fn(),
    onOpenPerson: vi.fn(),
    onOpenMessage: vi.fn(),
    onClose: vi.fn(),
    ...over,
  };
  render(
    <div>
      <p>outside</p>
      <ContactPopover {...props} />
    </div>,
  );
  return props;
}

describe("ContactPopover", () => {
  it("is a dialog at the clicked name, named by the person", () => {
    popover();
    const dialog = screen.getByRole("dialog", { name: "Ada Lovelace" });
    expect(dialog.style.left).toBe("120px");
    expect(dialog.style.top).toBe("80px");
    expect(dialog.className).toContain("w-[320px]");
    expect(within(dialog).getByText("ada@example.com").className).toContain("text-secondary");
    expect(within(dialog).getByText("Analytical Engines")).toBeTruthy();
  });

  it("shows the address it was opened with while the card loads", () => {
    popover({ card: null });
    expect(screen.getByRole("dialog", { name: "Ada L." })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Open in People" })).toBeNull();
  });

  it("names a stranger by the address when there is no name", () => {
    popover({
      address: { name: "", address: "x@example.com" },
      card: card({ email: "x@example.com", name: "", person: undefined }),
    });
    expect(screen.getByRole("dialog", { name: "x@example.com" })).toBeTruthy();
  });

  it("writes, opens the person, and opens recent mail", () => {
    const props = popover({ card: card({ recent: [msg(41, "Engines", new Date(2026, 9, 1, 9, 0))] }) });
    fireEvent.click(screen.getByRole("button", { name: "Message" }));
    expect(props.onCompose).toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Open in People" }));
    expect(props.onOpenPerson).toHaveBeenCalledWith(7);
    const recent = screen.getByRole("region", { name: "Recent Mail" });
    const row = within(recent).getByRole("button");
    expect(row.textContent).toBe(`Engines${formatListDate(new Date(2026, 9, 1, 9, 0), now)}`);
    fireEvent.click(row);
    expect(props.onOpenMessage).toHaveBeenCalledWith(41);
    expect(screen.queryByRole("button", { name: "Add to Contacts" })).toBeNull();
  });

  it("offers Add to Contacts for a stranger, and shows how it went", () => {
    const stranger = card({ person: undefined, canAdd: true, name: "Ada L." });
    const props = popover({ card: stranger });
    expect(screen.queryByRole("button", { name: "Open in People" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Add to Contacts" }));
    expect(props.onAdd).toHaveBeenCalled();
  });

  it.each<[ContactPopoverProps["add"], string]>([
    ["adding", "Adding…"],
    ["added", "Added"],
  ])("shows %s", (add, text) => {
    popover({ card: card({ person: undefined, canAdd: true }), add });
    expect(screen.getByText(text)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Add to Contacts" })).toBeNull();
  });

  it("shows a failed add and offers it again", () => {
    popover({ card: card({ person: undefined, canAdd: true }), add: { error: "the server refused" } });
    expect(screen.getByRole("alert").textContent).toBe("the server refused");
    expect(screen.getByRole("button", { name: "Add to Contacts" })).toBeTruthy();
  });

  it("closes with Escape and a click outside, not a click inside", () => {
    const props = popover();
    fireEvent.mouseDown(screen.getByText("ada@example.com"));
    expect(props.onClose).not.toHaveBeenCalled();
    fireEvent.mouseDown(screen.getByText("outside"));
    expect(props.onClose).toHaveBeenCalledTimes(1);
    fireEvent.keyDown(document, { key: "Escape" });
    expect(props.onClose).toHaveBeenCalledTimes(2);
  });
});
