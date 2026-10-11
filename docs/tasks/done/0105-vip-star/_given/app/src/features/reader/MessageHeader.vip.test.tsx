// CONTRACT TEST for task card T-0105 (docs/tasks). Do not edit.
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { Message } from "../../rpc/gen/api";
import { MessageHeader } from "./MessageHeader";

const message: Message = {
  summary: {
    id: 1,
    accountId: 1,
    mailboxIds: [1],
    threadId: 1,
    subject: "Offsite plan",
    from: { name: "Ann Smith", address: "ann@northwind.test" },
    date: new Date(2026, 9, 7, 9, 41).toISOString(),
    preview: "",
    flags: { seen: true, flagged: false, answered: false, forwarded: false, draft: false, flagColor: 0 },
    hasAttachments: false,
    size: 100,
    threadCount: 1,
  },
  to: [{ name: "Test One", address: "test1@mailtest.test" }],
  cc: [],
  replyTo: [],
  messageId: "a@x",
  inReplyTo: "",
  references: [],
  listId: "",
  listUnsubscribe: "",
  parts: [],
  bodyFetched: true,
};

describe("MessageHeader's VIP star", () => {
  it("follows a VIP sender's name", () => {
    render(<MessageHeader message={message} vip />);
    const header = screen.getByRole("banner");
    const star = within(header).getByRole("img", { name: "VIP" });
    const name = within(header).getByText("Ann Smith");
    expect(name.compareDocumentPosition(star) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("is absent for other senders", () => {
    render(<MessageHeader message={message} />);
    expect(screen.queryByRole("img", { name: "VIP" })).toBeNull();
  });
});
