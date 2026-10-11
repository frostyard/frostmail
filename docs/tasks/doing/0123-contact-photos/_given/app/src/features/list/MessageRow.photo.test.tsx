// CONTRACT TEST for task card T-0123 (docs/tasks). Do not edit.
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { avatarTone } from "../../lib/format";
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

function row(photo?: string | null) {
  render(
    <MessageRow
      message={summary}
      selected={false}
      focused={false}
      showThreadCount
      now={new Date(2026, 9, 7, 15, 0)}
      onSelect={vi.fn()}
      onContextMenu={vi.fn()}
      photo={photo}
    />,
  );
  return screen.getByRole("option");
}

describe("MessageRow's contact photo", () => {
  it("has no avatar without Contact Photos", () => {
    const el = row();
    expect(el.querySelector("[data-avatar]")).toBeNull();
    expect(el.className).toContain("pl-6");
  });

  it("shows the initials on their tone without a photo", () => {
    const el = row(null);
    const avatar = el.querySelector("[data-avatar]") as HTMLElement;
    expect(avatar.textContent).toBe("AS");
    expect(avatar.className).toContain(`bg-avatar-${avatarTone("ann@northwind.test")}`);
    expect(avatar.getAttribute("aria-hidden")).toBe("true");
    expect(el.className).toContain("pl-16");
  });

  it("shows the photo", () => {
    const el = row("data:image/png;base64,AAAA");
    const img = el.querySelector("[data-avatar] img") as HTMLImageElement;
    expect(img.getAttribute("src")).toBe("data:image/png;base64,AAAA");
  });
});
