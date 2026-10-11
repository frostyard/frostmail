// CONTRACT TEST for task card T-0127 (docs/tasks). Do not edit.
import { render, screen } from "@testing-library/react";
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
  preview: "Shall we hold the offsite in Lisbon this year?",
  flags: { seen: true, flagged: false, answered: false, forwarded: false, draft: false, flagColor: 0 },
  hasAttachments: true,
  size: 1234,
  threadCount: 1,
};

function row(message: MessageSummary) {
  render(
    <MessageRow
      message={message}
      selected={false}
      focused={false}
      showThreadCount
      now={new Date(2026, 9, 7, 15, 0)}
      onSelect={vi.fn()}
      onContextMenu={vi.fn()}
    />,
  );
}

describe("MessageRow muted", () => {
  it("shows a muted conversation's bell before the paperclip", () => {
    row({ ...summary, muted: true });
    const bell = screen.getByRole("img", { name: "Muted" });
    expect(bell.getAttribute("title")).toBe("Muted");
    expect(bell.className).toContain("text-secondary");
    expect(bell.querySelector("svg")?.getAttribute("width")).toBe("12");
    const clip = screen.getByLabelText("Has attachments");
    expect(bell.compareDocumentPosition(clip) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("shows no bell otherwise", () => {
    row(summary);
    expect(screen.queryByRole("img", { name: "Muted" })).toBeNull();
  });
});
