// The draft, outbox, identity and address domains of MockTransport:
// drafts kept in memory, and sends that go out when the undo delay ends,
// with the events maild emits (docs/design/send.md).
import {
  type Account,
  type Address,
  type Draft,
  type DraftContent,
  type DraftKind,
  ErrorCode,
  type Event,
  type Identity,
  type OutboxItem,
} from "../gen/api";
import { RPCError } from "../transport";

/** NOT_HANDLED is what MockCompose.dispatch returns for other domains. */
export const NOT_HANDLED = Symbol("not handled");

/** MockSource is what a reply or forward needs from a message. */
export interface MockSource {
  accountId: number;
  from: Address;
  to: Address[];
  cc: Address[];
  subject: string;
  html: string;
  text: string;
}

type Params = Record<string, unknown>;

function num(v: unknown): number {
  if (typeof v !== "number") throw new RPCError(ErrorCode.invalidParams, "expected a number");
  return v;
}

function notFound(what: string): RPCError {
  return new RPCError(ErrorCode.notFound, `${what} does not exist`);
}

const VALID = /^[^@\s]+@[^@\s.]+(\.[^@\s.]+)+$/;

/** MockCompose serves drafts and the outbox for MockTransport. */
export class MockCompose {
  private readonly drafts = new Map<number, Draft>();
  private readonly outbox = new Map<number, OutboxItem>();
  private readonly timers = new Map<number, ReturnType<typeof setTimeout>>();
  private readonly identityList: Identity[];
  private nextIdentity: number;
  private nextDraft = 1;
  private nextOutbox = 1;
  private nextAttachment = 1;

  constructor(
    private readonly accounts: Account[],
    private readonly emit: (event: Event) => void,
    private readonly source: (messageId: number) => MockSource | undefined,
    private readonly addresses: () => Address[],
    /** How long sent mail waits; the real default is 10 s. */
    readonly undoMs = 10_000,
  ) {
    this.nextIdentity = Math.max(0, ...accounts.map((a) => a.id)) + 1;
    this.identityList = accounts.map((a) => ({
      id: a.id,
      accountId: a.id,
      name: a.displayName,
      email: a.email,
      replyTo: "",
      signatureHtml: "",
      isDefault: true,
    }));
  }

  /** dispatch serves draft.*, outbox.*, identity.* and address.*, else returns NOT_HANDLED. */
  dispatch(method: string, p: Params): unknown {
    switch (method) {
      case "draft.create": {
        const d = this.create(
          p.kind as DraftKind,
          typeof p.accountId === "number" ? p.accountId : undefined,
          p.sourceId,
        );
        if (Array.isArray(p.to) && p.kind === "new") {
          this.draft(d.id).content.to = p.to as Address[];
          return structuredClone(this.draft(d.id));
        }
        return d;
      }
      case "draft.open": {
        const src = this.source(num(p.messageId));
        if (!src) throw notFound(`message ${num(p.messageId)}`);
        const d = this.draft(this.create("new", src.accountId, undefined).id);
        d.content.subject = src.subject;
        d.content.to = src.to;
        d.content.html = src.html !== "" ? src.html : `<p>${src.text}</p>`;
        return structuredClone(d);
      }
      case "draft.get":
        return structuredClone(this.draft(num(p.id)));
      case "draft.list":
        return [...this.drafts.values()].map((d) => structuredClone(d));
      case "draft.update": {
        const d = this.draft(num(p.id));
        d.content = structuredClone(p.content as DraftContent);
        this.touch(d);
        return structuredClone(d);
      }
      case "draft.attach": {
        const d = this.draft(num(p.id));
        const path = String(p.path);
        const a = {
          id: this.nextAttachment++,
          filename: path.split("/").pop() ?? path,
          contentType: "application/octet-stream",
          size: 1000,
        };
        d.attachments.push(a);
        this.touch(d);
        return a;
      }
      case "draft.detach": {
        const d = this.draft(num(p.id));
        const before = d.attachments.length;
        d.attachments = d.attachments.filter((a) => a.id !== p.attachmentId);
        if (d.attachments.length === before) throw notFound(`attachment ${String(p.attachmentId)}`);
        this.touch(d);
        return null;
      }
      case "draft.delete": {
        const d = this.draft(num(p.id));
        this.drafts.delete(d.id);
        this.emit({ event: "draft.changed", data: { id: d.id, accountId: d.accountId, deleted: true } });
        return null;
      }
      case "draft.send":
        return this.send(this.draft(num(p.id)), typeof p.sendAt === "string" ? p.sendAt : undefined);
      case "outbox.list":
        return [...this.outbox.values()].filter((o) => o.state !== "sent").map((o) => structuredClone(o));
      case "outbox.cancel":
        return this.cancel(num(p.id));
      case "outbox.reschedule": {
        const o = this.outbox.get(num(p.id));
        if (!o) throw notFound(`outbox message ${num(p.id)}`);
        if (o.state !== "queued" || !o.scheduled)
          throw new RPCError(ErrorCode.conflict, `message ${o.id} is not waiting to be sent later`);
        clearTimeout(this.timers.get(o.id));
        this.timers.delete(o.id);
        this.place(o, String(p.sendAt));
        return structuredClone(o);
      }
      case "outbox.retry": {
        const o = this.outbox.get(num(p.id));
        if (!o) throw notFound(`outbox message ${num(p.id)}`);
        if (o.state !== "failed") throw new RPCError(ErrorCode.conflict, `message ${o.id} has not failed`);
        this.queue(o, 0);
        return null;
      }
      case "identity.list":
        return this.identityList.filter((i) => typeof p.accountId !== "number" || i.accountId === p.accountId);
      case "identity.create": {
        const accountId = num(p.accountId);
        if (!this.accounts.some((a) => a.id === accountId)) throw notFound(`account ${accountId}`);
        const email = String(p.email);
        if (this.identityList.some((i) => i.accountId === accountId && i.email.toLowerCase() === email.toLowerCase())) {
          throw new RPCError(ErrorCode.conflict, `${email} is already an address of account ${accountId}`);
        }
        const identity: Identity = {
          id: this.nextIdentity++,
          accountId,
          email,
          name:
            typeof p.name === "string"
              ? p.name
              : (this.identityList.find((i) => i.accountId === accountId && i.isDefault)?.name ?? ""),
          replyTo: "",
          signatureHtml: "",
          isDefault: false,
        };
        this.identityList.push(identity);
        this.emit({ event: "account.changed", data: { id: accountId, deleted: false } });
        return { ...identity };
      }
      case "identity.delete": {
        const index = this.identityList.findIndex((i) => i.id === p.id);
        const identity = this.identityList[index];
        if (!identity) throw notFound(`identity ${String(p.id)}`);
        if (identity.isDefault) throw new RPCError(ErrorCode.conflict, "cannot remove the default identity");
        this.identityList.splice(index, 1);
        this.emit({ event: "account.changed", data: { id: identity.accountId, deleted: false } });
        return null;
      }
      case "identity.update": {
        const i = this.identityList.find((x) => x.id === p.id);
        if (!i) throw notFound(`identity ${String(p.id)}`);
        if (typeof p.name === "string") i.name = p.name;
        if (typeof p.replyTo === "string") i.replyTo = p.replyTo;
        if (typeof p.signatureHtml === "string") i.signatureHtml = p.signatureHtml;
        return { ...i };
      }
      case "address.suggest": {
        const prefix = String(p.prefix).toLowerCase();
        const limit = typeof p.limit === "number" && p.limit > 0 ? p.limit : 10;
        if (prefix === "") return [];
        const seen = new Set<string>();
        return this.addresses()
          .filter((a) => {
            const key = a.address.toLowerCase();
            if (seen.has(key)) return false;
            seen.add(key);
            const words = a.name.toLowerCase().split(/\s+/);
            return key.startsWith(prefix) || words.some((w) => w.startsWith(prefix));
          })
          .slice(0, limit);
      }
      default:
        return NOT_HANDLED;
    }
  }

  private draft(id: number): Draft {
    const d = this.drafts.get(id);
    if (!d) throw notFound(`draft ${id}`);
    return d;
  }

  private touch(d: Draft): void {
    d.updatedAt = new Date().toISOString();
    this.emit({ event: "draft.changed", data: { id: d.id, accountId: d.accountId, deleted: false } });
  }

  private create(kind: DraftKind, accountId: number | undefined, sourceId: unknown): Draft {
    const src = typeof sourceId === "number" ? this.source(sourceId) : undefined;
    if (kind !== "new" && !src) throw notFound(`message ${String(sourceId)}`);
    const acct = accountId ?? src?.accountId ?? this.accounts[0]?.id;
    if (acct === undefined) throw new RPCError(ErrorCode.invalidParams, "there is no account to write from");
    const content: DraftContent = { identityId: acct, to: [], cc: [], bcc: [], subject: "", html: "<p><br></p>" };
    if (src) {
      const quoted = src.html !== "" ? src.html : `<p>${src.text}</p>`;
      const name = src.from.name || src.from.address;
      if (kind === "forward") {
        content.subject = /^fwd?:/i.test(src.subject) ? src.subject : `Fwd: ${src.subject}`;
        content.html = `<p><br></p><p>Begin forwarded message:</p><blockquote type="cite">${quoted}</blockquote>`;
      } else {
        content.to = [src.from];
        if (kind === "replyall") content.cc = [...src.to, ...src.cc];
        content.subject = /^re:/i.test(src.subject) ? src.subject : `Re: ${src.subject}`;
        content.html = `<p><br></p><p>${name} wrote:</p><blockquote type="cite">${quoted}</blockquote>`;
      }
    }
    const d: Draft = {
      id: this.nextDraft++,
      accountId: acct,
      content,
      attachments: [],
      kind,
      updatedAt: new Date().toISOString(),
    };
    if (typeof sourceId === "number") d.sourceId = sourceId;
    this.drafts.set(d.id, d);
    this.emit({ event: "draft.changed", data: { id: d.id, accountId: d.accountId, deleted: false } });
    return structuredClone(d);
  }

  private send(d: Draft, sendAt?: string): OutboxItem {
    const all = [...d.content.to, ...d.content.cc, ...d.content.bcc];
    if (all.length === 0) throw new RPCError(ErrorCode.invalidParams, "the message has no recipients");
    const bad = all.find((a) => !VALID.test(a.address));
    if (bad) throw new RPCError(ErrorCode.invalidParams, `"${bad.address}" is not a valid address`);
    for (const o of this.outbox.values()) {
      if (o.draftId === d.id && o.state !== "failed" && o.state !== "sent") {
        throw new RPCError(ErrorCode.conflict, `draft ${d.id} is already being sent`);
      }
    }
    const o: OutboxItem = {
      id: this.nextOutbox++,
      accountId: d.accountId,
      draftId: d.id,
      subject: d.content.subject,
      to: d.content.to,
      state: "queued",
      scheduled: false,
      attempts: 0,
    };
    this.outbox.set(o.id, o);
    this.place(o, sendAt);
    return structuredClone(o);
  }

  /** place queues a message as maild's draft.send does: a sendAt past the
   *  undo delay is Send Later, due then; anything else waits the undo delay. */
  private place(o: OutboxItem, sendAt?: string): void {
    const at = sendAt === undefined ? Number.NaN : Date.parse(sendAt);
    if (at > Date.now() + this.undoMs) this.queue(o, at - Date.now(), true);
    else this.queue(o, this.undoMs);
  }

  private queue(o: OutboxItem, delay: number, scheduled = false): void {
    o.state = "queued";
    o.scheduled = scheduled;
    o.sendAt = new Date(Date.now() + delay).toISOString();
    this.changed(o, false);
    // setTimeout cannot wait longer than about 24 days; the mock never
    // sends such a message.
    if (delay > 2_147_483_647) return;
    this.timers.set(
      o.id,
      setTimeout(() => {
        this.timers.delete(o.id);
        o.state = "sent";
        o.scheduled = false;
        o.sendAt = undefined;
        if (o.draftId !== undefined) {
          this.drafts.delete(o.draftId);
          this.emit({ event: "draft.changed", data: { id: o.draftId, accountId: o.accountId, deleted: true } });
          o.draftId = undefined;
        }
        this.changed(o, false);
      }, delay),
    );
  }

  private cancel(id: number): Draft {
    const o = this.outbox.get(id);
    if (!o) throw notFound(`outbox message ${id}`);
    if (o.state !== "queued") throw new RPCError(ErrorCode.conflict, `message ${id} is already being sent`);
    clearTimeout(this.timers.get(id));
    this.timers.delete(id);
    this.outbox.delete(id);
    this.changed(o, true);
    return structuredClone(this.draft(o.draftId ?? -1));
  }

  private changed(o: OutboxItem, deleted: boolean): void {
    this.emit({ event: "outbox.changed", data: { id: o.id, accountId: o.accountId, state: o.state, deleted } });
  }
}
