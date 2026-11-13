// P-211: a row shows as many lines of preview as it is told.
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { MessageSummary } from "../../rpc/gen/api";
import { MessageRow } from "./MessageRow";

const message: MessageSummary = {
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

function row(previewLines?: number) {
  const view = render(
    <MessageRow
      message={message}
      selected={false}
      focused={false}
      showThreadCount
      now={new Date(2026, 9, 7, 15, 0)}
      onSelect={vi.fn()}
      onContextMenu={vi.fn()}
      previewLines={previewLines}
    />,
  );
  return { el: screen.getByRole("option"), preview: screen.queryByText(message.preview), ...view };
}

describe("MessageRow preview lines", () => {
  it("keeps the 84px row and its two-line clamp at 2 lines and when not told", () => {
    for (const lines of [undefined, 2]) {
      const { el, preview, unmount } = row(lines);
      expect(el.className).toContain("h-[84px]");
      expect(el.style.height).toBe("");
      expect(preview?.className).toContain("line-clamp-2");
      unmount();
    }
  });

  it("clamps the preview to the lines asked and sizes the row for them", () => {
    for (const [lines, height] of [
      [1, "68px"],
      [3, "100px"],
      [5, "132px"],
    ] as const) {
      const { el, preview, unmount } = row(lines);
      expect(el.style.height).toBe(height);
      expect(preview?.className).toContain(`line-clamp-${lines}`);
      expect(preview?.className).not.toContain("line-clamp-2");
      unmount();
    }
  });

  it("shows no preview at 0 lines, and keeps the sender and the subject", () => {
    const { el, preview } = row(0);
    expect(preview).toBeNull();
    expect(el.style.height).toBe("52px");
    expect(screen.getByText("Ann Smith")).toBeTruthy();
    expect(screen.getByText("Offsite plan")).toBeTruthy();
  });
});
