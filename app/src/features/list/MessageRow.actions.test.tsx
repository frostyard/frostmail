// CONTRACT TEST for task card T-0102 (docs/tasks). Do not edit.
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
    hasAttachments: true,
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
    onAction: vi.fn(),
    canArchive: true,
    ...over,
  };
  render(<MessageRow {...props} />);
  return { props, el: screen.getByRole("option") };
}

// The actions are mouse-only duplicates of the row's keyboard commands, so
// they are hidden from assistive technology: the tests find them by their
// data attributes, as a pointer does.
function actions(): HTMLElement {
  const found = document.querySelector<HTMLElement>("[data-actions]");
  if (!found) throw new Error("no row actions");
  return found;
}

function items(): HTMLElement[] {
  return [...actions().querySelectorAll<HTMLElement>("[data-action]")];
}

function item(action: string): HTMLElement {
  const found = actions().querySelector<HTMLElement>(`[data-action="${action}"]`);
  if (!found) throw new Error(`no ${action} action`);
  return found;
}

describe("MessageRow actions", () => {
  it("offers Flag, Archive and Delete in place of the date under the pointer", () => {
    const { el } = row();
    expect(el.className).toContain("group");
    const group = actions();
    expect(group.className).toContain("hidden");
    expect(group.className).toContain("group-hover:flex");
    expect(group.getAttribute("aria-hidden")).toBe("true");
    expect(items().map((b) => b.getAttribute("data-action"))).toEqual(["flag", "archive", "delete"]);
    // The date and the paperclip give way to the actions.
    const date = screen.getByText(formatListDate(new Date(2026, 9, 7, 9, 41), now));
    const meta = date.closest(".group-hover\\:hidden");
    expect(meta).not.toBeNull();
    expect(meta?.contains(screen.getByLabelText("Has attachments"))).toBe(true);
    expect(meta?.contains(group)).toBe(false);
  });

  it("sizes, tips and hides each action, and keeps it out of the tab order", () => {
    row();
    expect(items().map((b) => b.getAttribute("title"))).toEqual([
      "Flag (Ctrl+Shift+L)",
      "Archive (Ctrl+Alt+A)",
      "Delete (Delete)",
    ]);
    for (const b of items()) {
      expect(b.tagName).toBe("SPAN");
      expect(b.hasAttribute("tabindex")).toBe(false);
      expect(b.hasAttribute("role")).toBe(false);
      expect(b.className).toContain("h-[18px]");
      expect(b.className).toContain("w-[22px]");
      expect(b.className).toContain("text-secondary");
      expect(b.className).toContain("hover:bg-selection-inactive");
      expect(b.querySelector("svg")?.getAttribute("width")).toBe("14");
    }
  });

  it("acts on its message without selecting it", () => {
    const { props } = row();
    for (const action of ["flag", "archive", "delete"] as const) {
      fireEvent.click(item(action));
      expect(props.onAction).toHaveBeenLastCalledWith(42, action);
    }
    expect(props.onAction).toHaveBeenCalledTimes(3);
    expect(props.onSelect).not.toHaveBeenCalled();
  });

  it("keeps the pointer press from moving focus or opening a draft", () => {
    const onDoubleClick = vi.fn();
    const props: MessageRowProps = {
      message: summary(),
      selected: false,
      focused: false,
      showThreadCount: true,
      now,
      onSelect: vi.fn(),
      onContextMenu: vi.fn(),
      onAction: vi.fn(),
      canArchive: true,
    };
    render(
      // biome-ignore lint/a11y/noStaticElementInteractions: the list's wrapper opens drafts on double click.
      <div onDoubleClick={onDoubleClick}>
        <MessageRow {...props} />
      </div>,
    );
    const del = item("delete");
    expect(fireEvent.mouseDown(del)).toBe(false);
    fireEvent.doubleClick(del);
    expect(onDoubleClick).not.toHaveBeenCalled();
  });

  it("tips the flag action Unflag, filled, for a flagged message", () => {
    row({}, { flags: { ...summary().flags, flagged: true, flagColor: 2 } });
    const unflag = item("flag");
    expect(unflag.getAttribute("title")).toBe("Unflag (Ctrl+Shift+L)");
    expect(unflag.querySelector("svg")?.getAttribute("class")).toContain("fill-current");
  });

  it("leaves out Archive without an archive destination", () => {
    row({ canArchive: false });
    expect(items().map((b) => b.getAttribute("data-action"))).toEqual(["flag", "delete"]);
  });

  it("has no actions without onAction, and keeps the date visible", () => {
    row({ onAction: undefined });
    expect(document.querySelector("[data-actions]")).toBeNull();
    expect(
      screen.getByText(formatListDate(new Date(2026, 9, 7, 9, 41), now)).closest(".group-hover\\:hidden"),
    ).toBeNull();
  });

  it("uses the contrast color on a selected row in a focused list", () => {
    row({ selected: true, focused: true });
    for (const b of items()) {
      expect(b.className).toContain("text-accent-contrast");
      expect(b.className).toContain("hover:bg-accent-contrast/20");
      expect(b.className).not.toContain("text-secondary");
    }
  });
});
