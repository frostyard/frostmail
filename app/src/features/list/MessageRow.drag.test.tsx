// CONTRACT TEST for task card T-0121 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { MessageSummary } from "../../rpc/gen/api";
import { MessageRow } from "./MessageRow";

const summary: MessageSummary = {
  id: 42,
  accountId: 1,
  mailboxIds: [1],
  threadId: 7,
  subject: "Offsite plan",
  from: { name: "Ann Smith", address: "ann@northwind.test" },
  date: new Date(2026, 9, 7, 9, 41).toISOString(),
  preview: "Lisbon?",
  flags: { seen: true, flagged: false, answered: false, forwarded: false, draft: false, flagColor: 0 },
  hasAttachments: false,
  size: 1234,
  threadCount: 1,
};

function row(onDragStart?: (id: number) => void) {
  render(
    <MessageRow
      message={summary}
      selected={false}
      focused={false}
      showThreadCount
      now={new Date(2026, 9, 7, 15, 0)}
      onSelect={vi.fn()}
      onContextMenu={vi.fn()}
      onDragStart={onDragStart}
    />,
  );
  return screen.getByRole("option");
}

describe("MessageRow dragging", () => {
  it("drags when the list takes drags", () => {
    const onDragStart = vi.fn();
    const el = row(onDragStart);
    expect(el.getAttribute("draggable")).toBe("true");
    fireEvent.dragStart(el, { dataTransfer: { setData: vi.fn(), types: [] } });
    expect(onDragStart).toHaveBeenCalledWith(42, expect.anything());
  });

  it("does not drag otherwise", () => {
    expect(row().getAttribute("draggable")).toBe("false");
  });
});
