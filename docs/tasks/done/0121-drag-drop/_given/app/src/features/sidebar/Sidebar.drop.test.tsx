// CONTRACT TEST for task card T-0121 (docs/tasks). Do not edit.
import { createEvent, fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { SidebarSection } from "../../lib/mailboxTree";
import { Sidebar } from "./Sidebar";

const TYPE = "application/x-frostmail-messages";

const sections: SidebarSection[] = [
  {
    key: "account:1",
    title: "me@x.test",
    accountId: 1,
    items: [
      { key: "mailbox:10", label: "Inbox", icon: "inbox", depth: 0, unread: 0, selectable: true },
      { key: "mailbox:11", label: "Receipts", icon: "folder", depth: 0, unread: 0, selectable: true },
      { key: "path:1:Old", label: "Old", icon: "folder", depth: 0, unread: 0, selectable: false },
    ],
  },
];

function transfer(data: Record<string, string>) {
  return {
    types: Object.keys(data),
    getData: (type: string) => data[type] ?? "",
    setData: vi.fn(),
    dropEffect: "none",
    effectAllowed: "all",
  };
}

function sidebar() {
  const onDrop = vi.fn();
  render(
    <Sidebar
      sections={sections}
      selectedKey={null}
      focused={false}
      sync={{}}
      onSelect={vi.fn()}
      canDrop={(key) => key.startsWith("mailbox:")}
      onDrop={onDrop}
      dragType={TYPE}
    />,
  );
  return onDrop;
}

const row = (key: string) => document.querySelector<HTMLElement>(`[data-key="${key}"]`) as HTMLElement;

/** dragWith fires a drag event with a modifier held, which the test DOM's
 *  DragEvent does not take in its init. */
function dragWith(kind: "dragOver" | "drop", el: HTMLElement, dataTransfer: unknown, key: "altKey" | "ctrlKey") {
  const event = createEvent[kind](el, { dataTransfer });
  Object.defineProperty(event, key, { value: true });
  fireEvent(el, event);
}

describe("Sidebar drops", () => {
  it("takes dragged messages on a mailbox row, showing the target", () => {
    const onDrop = sidebar();
    const dt = transfer({ [TYPE]: '{"accountId":1,"ids":[5]}' });
    const taken = !fireEvent.dragOver(row("mailbox:11"), { dataTransfer: dt });
    expect(taken).toBe(true);
    expect(row("mailbox:11").className).toContain("bg-selection-inactive");
    fireEvent.drop(row("mailbox:11"), { dataTransfer: dt });
    expect(onDrop).toHaveBeenCalledWith("mailbox:11", '{"accountId":1,"ids":[5]}', false);
    expect(row("mailbox:11").className).not.toContain("bg-selection-inactive");
  });

  it("copies with Alt or Ctrl held", () => {
    const onDrop = sidebar();
    const dt = transfer({ [TYPE]: '{"accountId":1,"ids":[5]}' });
    dragWith("dragOver", row("mailbox:10"), dt, "altKey");
    dragWith("drop", row("mailbox:10"), dt, "altKey");
    expect(onDrop).toHaveBeenLastCalledWith("mailbox:10", '{"accountId":1,"ids":[5]}', true);
    dragWith("drop", row("mailbox:11"), dt, "ctrlKey");
    expect(onDrop).toHaveBeenLastCalledWith("mailbox:11", '{"accountId":1,"ids":[5]}', true);
  });

  it("leaves other rows and other drags alone", () => {
    const onDrop = sidebar();
    const other = transfer({ "text/plain": "hello" });
    expect(!fireEvent.dragOver(row("mailbox:11"), { dataTransfer: other })).toBe(false);
    fireEvent.drop(row("mailbox:11"), { dataTransfer: other });
    const messages = transfer({ [TYPE]: "{}" });
    expect(!fireEvent.dragOver(row("path:1:Old"), { dataTransfer: messages })).toBe(false);
    fireEvent.drop(row("path:1:Old"), { dataTransfer: messages });
    expect(onDrop).not.toHaveBeenCalled();
    expect(screen.getByRole("tree")).toBeTruthy();
  });
});
