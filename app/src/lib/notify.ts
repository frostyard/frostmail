import { ErrorCode, type InviteNotice, type NoticeKind, NoticeKindValues } from "../rpc/gen/api";
import { RPCError } from "../rpc/transport";

/** noticeOf reads a valid invite notice from a notifyRequired error. */
export function noticeOf(err: unknown): InviteNotice | null {
  if (!(err instanceof RPCError) || err.code !== ErrorCode.notifyRequired) return null;
  const data = err.data;
  if (typeof data !== "object" || data === null || !("kind" in data) || !("count" in data)) return null;
  const kind = NoticeKindValues.find((value) => value === data.kind);
  const count = data.count;
  if (kind === undefined || typeof count !== "number" || !Number.isInteger(count) || count < 1) return null;
  return { kind, count };
}

/** noticeTitle names the messages that will go to invitees. */
export function noticeTitle(kind: NoticeKind): string {
  const titles: Record<NoticeKind, string> = {
    invitations: "Send Invitations?",
    updates: "Send Updates?",
    cancellations: "Send Cancellations?",
  };
  return titles[kind];
}

/** noticeText explains how many invitees will receive email. */
export function noticeText(count: number): string {
  return count === 1 ? "The invitee will get it by email." : `${count} invitees will get it by email.`;
}

/** withNotify retries a write with notify only after the user agrees. */
export async function withNotify<T>(
  send: (notify?: true) => Promise<T>,
  ask: (notice: InviteNotice) => Promise<boolean>,
): Promise<T | null> {
  try {
    return await send(undefined);
  } catch (err) {
    const notice = noticeOf(err);
    if (notice === null) throw err;
    return (await ask(notice)) ? send(true) : null;
  }
}
