// CONTRACT TEST for task card T-0156 (docs/tasks). Do not edit.
import { describe, expect, it, vi } from "vitest";

import { ErrorCode } from "../rpc/gen/api";
import { RPCError } from "../rpc/transport";
import { noticeOf, noticeText, noticeTitle, withNotify } from "./notify";

const required = (data: unknown) => new RPCError(ErrorCode.notifyRequired, "the change tells invitees", data);

describe("noticeOf", () => {
  it("reads the notice from notifyRequired", () => {
    expect(noticeOf(required({ kind: "updates", count: 3 }))).toEqual({ kind: "updates", count: 3 });
    expect(noticeOf(required({ kind: "invitations", count: 1, more: true }))).toEqual({
      kind: "invitations",
      count: 1,
    });
  });

  it("is null for anything else", () => {
    expect(noticeOf(new RPCError(ErrorCode.invalidParams, "no", { kind: "updates", count: 3 }))).toBeNull();
    expect(noticeOf(new Error("boom"))).toBeNull();
    expect(noticeOf(undefined)).toBeNull();
    expect(noticeOf(required(undefined))).toBeNull();
    expect(noticeOf(required({ kind: "reminders", count: 2 }))).toBeNull();
    expect(noticeOf(required({ kind: "updates", count: 0 }))).toBeNull();
    expect(noticeOf(required({ kind: "updates", count: 1.5 }))).toBeNull();
    expect(noticeOf(required({ kind: "updates", count: "2" }))).toBeNull();
  });
});

describe("noticeTitle and noticeText", () => {
  it("name what goes out and to how many", () => {
    expect(noticeTitle("invitations")).toBe("Send Invitations?");
    expect(noticeTitle("updates")).toBe("Send Updates?");
    expect(noticeTitle("cancellations")).toBe("Send Cancellations?");
    expect(noticeText(1)).toBe("The invitee will get it by email.");
    expect(noticeText(4)).toBe("4 invitees will get it by email.");
  });
});

describe("withNotify", () => {
  it("sends once when nobody is told", async () => {
    const send = vi.fn(async (notify?: true) => (notify ? "told" : "quiet"));
    const ask = vi.fn(async () => true);
    await expect(withNotify(send, ask)).resolves.toBe("quiet");
    expect(send).toHaveBeenCalledTimes(1);
    expect(send).toHaveBeenCalledWith(undefined);
    expect(ask).not.toHaveBeenCalled();
  });

  it("asks, then sends again with notify", async () => {
    const send = vi.fn(async (notify?: true) => {
      if (!notify) throw required({ kind: "cancellations", count: 2 });
      return "sent";
    });
    const ask = vi.fn(async () => true);
    await expect(withNotify(send, ask)).resolves.toBe("sent");
    expect(ask).toHaveBeenCalledWith({ kind: "cancellations", count: 2 });
    expect(send.mock.calls).toEqual([[undefined], [true]]);
  });

  it("abandons the change when the user cancels", async () => {
    const send = vi.fn(async (notify?: true) => {
      if (!notify) throw required({ kind: "updates", count: 1 });
      return "sent";
    });
    await expect(withNotify(send, async () => false)).resolves.toBeNull();
    expect(send).toHaveBeenCalledTimes(1);
  });

  it("passes other errors through, without asking", async () => {
    const ask = vi.fn(async () => true);
    const refused = new RPCError(ErrorCode.conflict, "read-only");
    await expect(withNotify(async () => Promise.reject(refused), ask)).rejects.toBe(refused);
    expect(ask).not.toHaveBeenCalled();
    const later = new RPCError(ErrorCode.unavailable, "offline");
    const send = vi.fn(async (notify?: true) => {
      if (!notify) throw required({ kind: "updates", count: 1 });
      throw later;
    });
    await expect(withNotify(send, async () => true)).rejects.toBe(later);
  });
});
