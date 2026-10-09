// CONTRACT TEST for task card T-0065 (docs/tasks). Do not edit.
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Message } from "../../rpc/gen/api";
import { MessageHeader } from "./MessageHeader";

const message: Message = {
  summary: {
    id: 1,
    accountId: 1,
    mailboxIds: [1],
    threadId: 1,
    subject: "Plan",
    from: { name: "Ann Smith", address: "ann@northwind.test" },
    date: new Date(2026, 9, 7, 9, 41).toISOString(),
    preview: "",
    flags: { seen: true, flagged: false, answered: false, forwarded: false, draft: false, flagColor: 0 },
    hasAttachments: false,
    size: 1,
    threadCount: 1,
  },
  to: [
    { name: "Bob", address: "bob@x.test" },
    { name: "", address: "carol@x.test" },
    { name: "Dan", address: "dan@x.test" },
    { name: "Eve", address: "eve@x.test" },
    { name: "Fay", address: "fay@x.test" },
  ],
  cc: [{ name: "Gus", address: "gus@x.test" }],
  replyTo: [],
  messageId: "1@x",
  inReplyTo: "",
  references: [],
  listId: "",
  listUnsubscribe: "",
  parts: [],
  remoteContent: 0,
  trackersBlocked: 0,
} as unknown as Message;

describe("MessageHeader names", () => {
  it("makes the sender and recipients buttons that open the contact card", () => {
    const onAddress = vi.fn();
    render(<MessageHeader message={message} onAddress={onAddress} />);
    const sender = screen.getByRole("button", { name: "ann@northwind.test" });
    expect(sender.textContent).toBe("Ann Smith");
    expect(sender.className).toContain("text-reader-sender");
    fireEvent.click(sender);
    expect(onAddress).toHaveBeenLastCalledWith({ name: "Ann Smith", address: "ann@northwind.test" }, { x: 0, y: 0 });
    for (const a of ["bob@x.test", "carol@x.test", "dan@x.test", "gus@x.test"]) {
      expect(screen.getByRole("button", { name: a })).toBeTruthy();
    }
    expect(screen.queryByRole("button", { name: "eve@x.test" })).toBeNull();
    expect(screen.getByRole("button", { name: "carol@x.test" }).textContent).toBe("carol@x.test");
    const to = screen.getByRole("button", { name: "bob@x.test" }).parentElement;
    expect(to?.textContent).toBe("To: Bob, carol@x.test, Dan & 2 more");
    fireEvent.click(screen.getByRole("button", { name: "gus@x.test" }));
    expect(onAddress).toHaveBeenLastCalledWith({ name: "Gus", address: "gus@x.test" }, { x: 0, y: 0 });
  });

  it("keeps plain text without onAddress", () => {
    render(<MessageHeader message={message} />);
    expect(screen.queryAllByRole("button")).toHaveLength(0);
    expect(screen.getByText("To: Bob, carol@x.test, Dan & 2 more")).toBeTruthy();
  });
});
