// CONTRACT TEST for task card T-0105 (docs/tasks). Do not edit.
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
  hasAttachments: false,
  size: 1234,
  threadCount: 1,
};

function row(vip: boolean | undefined, selected = false, focused = false) {
  render(
    <MessageRow
      message={summary}
      selected={selected}
      focused={focused}
      showThreadCount
      now={new Date(2026, 9, 7, 15, 0)}
      onSelect={vi.fn()}
      onContextMenu={vi.fn()}
      vip={vip}
    />,
  );
}

describe("MessageRow's VIP star", () => {
  it("comes before a VIP sender's name", () => {
    row(true);
    const star = screen.getByRole("img", { name: "VIP" });
    const name = screen.getByText("Ann Smith");
    expect(star.compareDocumentPosition(name) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(star.getAttribute("class")).toContain("text-secondary");
  });

  it("takes the selection's contrast", () => {
    row(true, true, true);
    expect(screen.getByRole("img", { name: "VIP" }).getAttribute("class")).toContain("text-accent-contrast");
  });

  it("is absent for other senders", () => {
    row(undefined);
    expect(screen.queryByRole("img", { name: "VIP" })).toBeNull();
  });
});
