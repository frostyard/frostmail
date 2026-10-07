// CONTRACT TEST for task card T-0029 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { formatListDate } from "../../lib/format";
import type { MessageSummary } from "../../rpc/gen/api";
import { MessageRow, type MessageRowProps } from "./MessageRow";

const now = new Date(2026, 9, 7, 15, 0);

function summary(over: Partial<MessageSummary> = {}): MessageSummary {
  return {
    id: 42,
    accountId: 1,
    mailboxIds: [1],
    threadId: 7,
    subject: "Offsite plan",
    from: { name: "Ann Smith", address: "ann@northwind.test" },
    date: new Date(2026, 9, 7, 9, 41).toISOString(),
    preview: "Shall we hold the offsite in Lisbon this year?",
    flags: { seen: true, flagged: false, answered: false, forwarded: false, draft: false, flagColor: 0 },
    hasAttachments: false,
    size: 1234,
    threadCount: 1,
    ...over,
  };
}

function row(over: Partial<MessageRowProps> = {}, msg: Partial<MessageSummary> = {}) {
  const props: MessageRowProps = {
    message: summary(msg),
    selected: false,
    focused: false,
    showThreadCount: true,
    now,
    onSelect: vi.fn(),
    onContextMenu: vi.fn(),
    ...over,
  };
  render(<MessageRow {...props} />);
  return { props, el: screen.getByRole("option") };
}

describe("MessageRow", () => {
  it("shows sender, date, subject and preview in an 84px option", () => {
    const { el } = row();
    expect(el.getAttribute("data-message-id")).toBe("42");
    expect(el.getAttribute("tabindex")).toBe("-1");
    expect(el.className).toContain("h-[84px]");
    expect(screen.getByText("Ann Smith").className).toContain("text-list-sender");
    const date = formatListDate(new Date(2026, 9, 7, 9, 41), now);
    expect(screen.getByText(date).className).toContain("text-secondary");
    expect(screen.getByText("Offsite plan").className).toContain("text-list-subject");
    const preview = screen.getByText("Shall we hold the offsite in Lisbon this year?");
    expect(preview.className).toContain("line-clamp-2");
    expect(preview.className).toContain("text-list-preview");
  });

  it("falls back to the address and to (No Subject)", () => {
    row({}, { from: { name: "", address: "bob@acme.test" }, subject: "  " });
    expect(screen.getByText("bob@acme.test")).toBeTruthy();
    expect(screen.getByText("(No Subject)").className).toContain("text-tertiary");
  });

  it("marks unread messages with a dot", () => {
    row({}, { flags: { ...summary().flags, seen: false } });
    expect(screen.getByLabelText("Unread").className).toContain("bg-accent");
  });

  it("has no dot, flag, paperclip or badge for a plain read message", () => {
    row();
    expect(screen.queryByLabelText("Unread")).toBeNull();
    expect(screen.queryByLabelText(/^Flagged/)).toBeNull();
    expect(screen.queryByLabelText("Has attachments")).toBeNull();
    expect(screen.queryByLabelText(/messages$/)).toBeNull();
  });

  it("shows the flag in its color", () => {
    row({}, { flags: { ...summary().flags, flagged: true, flagColor: 3 } });
    const flag = screen.getByLabelText("Flagged Yellow");
    expect(flag.getAttribute("class")).toContain("text-flag-3");
  });

  it("shows a paperclip for attachments", () => {
    row({}, { hasAttachments: true });
    expect(screen.getByLabelText("Has attachments")).toBeTruthy();
  });

  it("shows the thread count in conversation mode only", () => {
    row({}, { threadCount: 3 });
    const badge = screen.getByLabelText("3 messages");
    expect(badge.textContent).toBe("3");
    expect(badge.className).toContain("bg-badge");
  });

  it("hides the thread count outside conversation mode", () => {
    row({ showThreadCount: false }, { threadCount: 3 });
    expect(screen.queryByLabelText("3 messages")).toBeNull();
  });

  it("shows selection in the accent color while the list has focus", () => {
    const { el } = row({ selected: true, focused: true }, { flags: { ...summary().flags, seen: false } });
    expect(el.getAttribute("aria-selected")).toBe("true");
    expect(el.className).toContain("bg-accent");
    expect(el.className).toContain("text-accent-contrast");
    expect(screen.getByLabelText("Unread").className).toContain("bg-accent-contrast");
  });

  it("shows selection in gray without focus", () => {
    const { el } = row({ selected: true, focused: false });
    expect(el.className).toContain("bg-selection-inactive");
    expect(el.className).not.toContain("bg-accent");
  });

  it("is not highlighted when not selected", () => {
    const { el } = row({ focused: true });
    expect(el.getAttribute("aria-selected")).toBe("false");
    expect(el.className).not.toContain("bg-accent");
    expect(el.className).not.toContain("bg-selection-inactive");
  });

  it("selects on click, toggles with Ctrl and extends with Shift", () => {
    const { props, el } = row();
    fireEvent.click(el);
    fireEvent.click(el, { ctrlKey: true });
    fireEvent.click(el, { shiftKey: true });
    expect(props.onSelect).toHaveBeenNthCalledWith(1, 42, "replace");
    expect(props.onSelect).toHaveBeenNthCalledWith(2, 42, "toggle");
    expect(props.onSelect).toHaveBeenNthCalledWith(3, 42, "range");
  });

  it("opens the context menu where the pointer is", () => {
    const { props, el } = row();
    const ev = fireEvent.contextMenu(el, { clientX: 120, clientY: 340 });
    expect(ev).toBe(false);
    expect(props.onContextMenu).toHaveBeenCalledWith(42, 120, 340);
  });
});
