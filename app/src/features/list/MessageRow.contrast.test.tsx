// lodge-ui ui/v0.7.0 darkened the light --text-tertiary (#6c6c71) and
// lightened the dark one (#95959a): on --accent it reaches only 1.26:1 and
// 1.33:1. A selected row in a focused list is accent, so its placeholder
// subject takes the row's contrast colour, not tertiary.
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { MessageSummary } from "../../rpc/gen/api";
import { MessageRow } from "./MessageRow";

const summary: MessageSummary = {
  id: 42,
  accountId: 1,
  mailboxIds: [1],
  threadId: 7,
  subject: "  ",
  from: { name: "Ann Smith", address: "ann@northwind.test" },
  date: new Date(2026, 9, 7, 9, 41).toISOString(),
  preview: "Shall we hold the offsite in Lisbon this year?",
  flags: { seen: true, flagged: false, answered: false, forwarded: false, draft: false, flagColor: 0 },
  hasAttachments: false,
  size: 1234,
  threadCount: 1,
};

function row(selected: boolean, focused: boolean) {
  render(
    <MessageRow
      message={summary}
      selected={selected}
      focused={focused}
      showThreadCount
      now={new Date(2026, 9, 7, 15, 0)}
      onSelect={vi.fn()}
      onContextMenu={vi.fn()}
    />,
  );
  return screen.getByText("(No Subject)").className;
}

describe("MessageRow's (No Subject) on the accent row", () => {
  it("is not tertiary on a selected row in a focused list", () => {
    const className = row(true, true);
    expect(screen.getByRole("option").className).toContain("text-accent-contrast");
    expect(className).not.toContain("text-tertiary");
  });

  it("stays tertiary on a selected row in an unfocused list", () => {
    expect(row(true, false)).toContain("text-tertiary");
  });
});
