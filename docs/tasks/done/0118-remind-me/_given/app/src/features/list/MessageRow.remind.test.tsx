// CONTRACT TEST for task card T-0118 (docs/tasks). Do not edit.
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { appLocale } from "../../lib/calendarDates";
import { whenText } from "../../lib/later";
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

const now = new Date(2026, 9, 7, 15, 0);

function row(message: MessageSummary) {
  render(
    <MessageRow
      message={message}
      selected={false}
      focused={false}
      showThreadCount
      now={now}
      onSelect={vi.fn()}
      onContextMenu={vi.fn()}
    />,
  );
}

describe("MessageRow's reminder clock", () => {
  it("shows a pending reminder between the paperclip and the date", () => {
    const at = new Date(2026, 9, 8, 8, 0).toISOString();
    row({ ...summary, remindAt: at });
    const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    const label = `Reminder ${whenText(new Date(at), now, zone, appLocale(navigator.language))}`;
    const clock = screen.getByRole("img", { name: label });
    expect(clock.getAttribute("title")).toBe(label);
    expect(clock.getAttribute("class")).toContain("text-secondary");
    const clip = screen.getByLabelText("Has attachments");
    expect(clip.compareDocumentPosition(clock) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    const date = screen.getByText(/9:41/);
    expect(clock.compareDocumentPosition(date) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("is not there without a reminder", () => {
    row(summary);
    expect(screen.queryByRole("img", { name: /^Reminder / })).toBeNull();
  });
});
