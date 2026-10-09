// MockTransport: an in-memory maild for component tests and for running the
// UI in a browser (VITE_MOCK=1). It implements the methods the app uses with
// maild's semantics: views keep ID snapshots and send view.delta after
// changes, flag and move calls emit message.changed and mailbox.changed.
import {
  type Account,
  type AccountCreateParams,
  type AccountUpdateParams,
  type Address,
  type Discovery,
  ErrorCode,
  type Event,
  type FlagChanges,
  type Flags,
  type Mailbox,
  type Message,
  type MessageSummary,
  type Part,
  type Rendering,
  type SyncStatus,
  type ViewQuery,
} from "../gen/api";
import { RPCError, type Transport } from "../transport";
import { MockCalendar, type MockCalendarData } from "./calendar";
import { MockCompose, NOT_HANDLED } from "./compose";
import { diffIds } from "./diff";
import { MockPeople, type MockPeopleData } from "./people";
import { MockTasks, type MockTasksData } from "./tasks";

/** MockMessage is a stored message: its summary plus what the reader shows. */
export interface MockMessage {
  summary: MessageSummary;
  to: Address[];
  cc: Address[];
  parts: Part[];
  text: string;
  /** Sanitized HTML as maild would return it; "" for text-only messages. */
  html: string;
  /** Remote images left out until render is called with remote. */
  remote: number;
  trackers: number;
}

/** MockMailbox is a mailbox without counts; the mock computes them. */
export type MockMailbox = Omit<Mailbox, "total" | "unread">;

/** MockData is everything the mock serves. */
export interface MockData {
  accounts: Account[];
  mailboxes: MockMailbox[];
  messages: MockMessage[];
  /** Address books and people; none when absent. */
  pim?: MockPeopleData;
  /** Events in pim's calendars; none when absent. */
  calendar?: MockCalendarData;
  /** Tasks in pim's task lists; none when absent. */
  tasks?: MockTasksData;
}

/** MockOptions tune the mock. */
export interface MockOptions {
  /** Milliseconds before each reply; 0 replies on the next microtask. */
  latency?: number;
  /** How long sent mail waits for undo; 10 s by default. */
  undoMs?: number;
}

interface MockView {
  query: ViewQuery;
  ids: number[];
}

type Params = Record<string, unknown>;

/** MockTransport serves MockData through the Transport interface. */
export class MockTransport implements Transport {
  /** Every call, for assertions. */
  readonly calls: { method: string; params: unknown }[] = [];
  private readonly messages = new Map<number, MockMessage>();
  private readonly mailboxes: MockMailbox[];
  private readonly accounts: Account[];
  private readonly views = new Map<number, MockView>();
  private nextView = 1;
  private readonly eventHandlers = new Set<(event: Event) => void>();
  private readonly closeHandlers = new Set<(reason: string) => void>();
  private closed: string | undefined;
  private readonly compose: MockCompose;
  private readonly people: MockPeople;
  private readonly calendar: MockCalendar;
  private readonly tasks: MockTasks;

  constructor(
    data: MockData,
    private readonly opts: MockOptions = {},
  ) {
    this.accounts = data.accounts;
    this.mailboxes = data.mailboxes;
    for (const m of data.messages) this.messages.set(m.summary.id, m);
    this.compose = new MockCompose(
      this.accounts,
      (e) => this.emit(e),
      (id) => {
        const m = this.messages.get(id);
        if (!m) return undefined;
        const s = m.summary;
        return {
          accountId: s.accountId,
          from: s.from,
          to: m.to,
          cc: m.cc,
          subject: s.subject,
          html: m.html,
          text: m.text,
        };
      },
      () => [...this.messages.values()].flatMap((m) => [m.summary.from, ...m.to, ...m.cc]),
      opts.undoMs,
    );
    const pim = data.pim ?? { collections: [], people: [] };
    this.calendar = new MockCalendar(
      data.calendar ?? { events: [], now: new Date().toISOString() },
      pim.collections,
      (e) => this.emit(e),
    );
    this.tasks = new MockTasks(data.tasks ?? { tasks: [], now: new Date().toISOString() }, pim.collections, (e) =>
      this.emit(e),
    );
    this.people = new MockPeople(
      pim,
      (e) => this.emit(e),
      () => [...this.messages.values()],
      () => this.accounts,
      (email) => this.calendar.upcoming(email),
    );
  }

  call<T>(method: string, params: unknown): Promise<T> {
    this.calls.push({ method, params });
    if (this.closed !== undefined) return Promise.reject(new Error(`maild connection closed: ${this.closed}`));
    return this.later(() => this.dispatch(method, (params ?? {}) as Params) as T);
  }

  onEvent(handler: (event: Event) => void): () => void {
    this.eventHandlers.add(handler);
    return () => this.eventHandlers.delete(handler);
  }

  onClose(handler: (reason: string) => void): () => void {
    if (this.closed !== undefined) {
      handler(this.closed);
      return () => {};
    }
    this.closeHandlers.add(handler);
    return () => this.closeHandlers.delete(handler);
  }

  /** close simulates a lost connection. */
  close(reason: string): void {
    if (this.closed !== undefined) return;
    this.closed = reason;
    for (const h of this.closeHandlers) h(reason);
    this.closeHandlers.clear();
  }

  /** createAccount adds an account as maild does: it emits account.changed, and account.list shows it after. */
  private createAccount(p: AccountCreateParams): Account {
    const server = (port: number): Account["imap"] => ({ host: "", port, tls: "tls", username: p.email });
    const a: Account = {
      id: Math.max(0, ...this.accounts.map((x) => x.id)) + 1,
      kind: p.kind,
      email: p.email,
      displayName: p.displayName,
      auth: p.auth,
      imap: p.imap ?? server(993),
      smtp: p.smtp ?? server(465),
      createdAt: new Date().toISOString(),
      readOnly: p.readOnly ?? false,
      notify: p.notify ?? true,
      syncDays: p.syncDays ?? 0,
      signedIn: p.auth === "password",
    };
    this.accounts.push(a);
    this.accountChanged(a.id);
    return a;
  }

  private discoverAccount(email: string): Discovery {
    const domain = email.split("@")[1]?.toLowerCase();
    if (domain === "gmail.com" || domain === "googlemail.com")
      return { kind: "gmail", source: "profile", auth: ["oauth2", "password"] };
    if (domain === "icloud.com" || domain === "me.com")
      return { kind: "icloud", source: "profile", auth: ["password"] };
    return { kind: "imap", source: "none", auth: ["password"] };
  }

  private updateAccount(p: AccountUpdateParams): Account {
    const a = this.account(p.id);
    if (p.displayName !== undefined) a.displayName = p.displayName;
    if (p.imap) a.imap = p.imap;
    if (p.smtp) a.smtp = p.smtp;
    if (p.readOnly !== undefined) a.readOnly = p.readOnly;
    if (p.notify !== undefined) a.notify = p.notify;
    if (p.syncDays !== undefined) a.syncDays = p.syncDays;
    this.accountChanged(a.id);
    return { ...a };
  }

  private account(id: number): Account {
    const a = this.accounts.find((x) => x.id === id);
    if (!a) throw new RPCError(ErrorCode.notFound, `account ${id} not found`);
    return a;
  }

  private accountChanged(id: number): void {
    this.emit({ event: "account.changed", data: { id, deleted: false } });
  }

  /** emit sends an event to subscribers on the next microtask. */
  emit(event: Event): void {
    queueMicrotask(() => {
      for (const h of this.eventHandlers) h(event);
    });
  }

  /** message returns a stored message, for assertions. */
  message(id: number): MockMessage | undefined {
    return this.messages.get(id);
  }

  /** add stores new messages as if sync had found them. */
  add(msgs: MockMessage[]): void {
    for (const m of msgs) this.messages.set(m.summary.id, m);
    this.changed(msgs.map((m) => m.summary));
  }

  /** update changes stored messages as if the server had. */
  update(ids: number[], fn: (m: MockMessage) => void): void {
    const touched: MessageSummary[] = [];
    for (const id of ids) {
      const m = this.messages.get(id);
      if (!m) continue;
      fn(m);
      touched.push(m.summary);
    }
    this.changed(touched);
  }

  /** remove deletes messages as if the server had expunged them. */
  remove(ids: number[]): void {
    const byAccount = new Map<number, number[]>();
    const mailboxes = new Set<number>();
    for (const id of ids) {
      const m = this.messages.get(id);
      if (!m) continue;
      this.messages.delete(id);
      byAccount.set(m.summary.accountId, [...(byAccount.get(m.summary.accountId) ?? []), id]);
      for (const mb of m.summary.mailboxIds) mailboxes.add(mb);
    }
    this.refreshViews();
    for (const [accountId, removed] of byAccount) {
      this.emit({ event: "message.removed", data: { accountId, ids: removed } });
    }
    this.mailboxesChanged(mailboxes);
  }

  private later<T>(fn: () => T): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      const run = () => {
        try {
          resolve(fn());
        } catch (err) {
          reject(err);
        }
      };
      if (this.opts.latency) setTimeout(run, this.opts.latency);
      else queueMicrotask(run);
    });
  }

  private dispatch(method: string, p: Params): unknown {
    switch (method) {
      case "rpc.hello":
        return { protocol: 1, server: "mock" };
      case "events.subscribe":
        return { seq: 0, resync: false };
      case "account.list":
        return this.accounts.map((a) => ({ ...a })); // a new list each call, as from maild
      case "account.discover":
        return this.discoverAccount(String(p.email ?? ""));
      case "account.authorize": {
        const a = this.account(num(p.id));
        if (a.auth === "password") throw new RPCError(ErrorCode.invalidParams, "account uses password sign-in");
        this.people.authorize(a.id);
        a.signedIn = true;
        this.accountChanged(a.id);
        return { url: `https://accounts.google.test/authorize/${a.id}` };
      }
      case "account.create":
        return this.createAccount(p as unknown as AccountCreateParams);
      case "account.update":
        return this.updateAccount(p as unknown as AccountUpdateParams);
      case "account.setPassword":
        this.accountChanged(this.account(num(p.id)).id);
        return null;
      case "account.delete": {
        const a = this.account(num(p.id));
        this.accounts.splice(this.accounts.indexOf(a), 1);
        this.emit({ event: "account.changed", data: { id: a.id, deleted: true } });
        return null;
      }
      case "mailbox.list":
        return this.mailboxList(typeof p.accountId === "number" ? p.accountId : undefined);
      case "sync.status":
        return this.accounts.map((a) => idleStatus(a.id));
      case "sync.now":
        this.syncNow(num(p.accountId));
        return null;
      case "message.get":
        return this.get(num(p.id));
      case "message.body": {
        const m = this.find(num(p.id));
        return { text: m.text, hasHtml: m.html !== "" };
      }
      case "message.summaries":
        return ids(p.ids).flatMap((id) => {
          const m = this.messages.get(id);
          return m ? [m.summary] : [];
        });
      case "message.render":
        return this.render(num(p.id), p.remote === true);
      case "message.part":
        return this.part(num(p.id), String(p.path));
      case "message.setFlags":
        this.setFlags(ids(p.ids), (p.changes ?? {}) as FlagChanges);
        return null;
      case "message.move":
        this.move(ids(p.ids), num(p.mailboxId));
        return null;
      case "message.delete":
        this.delete(ids(p.ids));
        return null;
      case "thread.messages":
        return this.thread(num(p.id));
      case "view.open":
        return this.openView((p.query ?? {}) as ViewQuery);
      case "view.range":
        return this.range(num(p.id), num(p.start), num(p.end));
      case "view.close":
        if (!this.views.delete(num(p.id))) throw notFound(`view ${num(p.id)} does not exist`);
        return null;
      default: {
        const r = this.compose.dispatch(method, p);
        if (r !== NOT_HANDLED) return r;
        const calendar = this.calendar.dispatch(method, p);
        if (calendar !== NOT_HANDLED) return calendar;
        const tasks = this.tasks.dispatch(method, p);
        if (tasks !== NOT_HANDLED) return tasks;
        const q = this.people.dispatch(method, p);
        if (q !== NOT_HANDLED) return q;
        throw new RPCError(ErrorCode.methodNotFound, `method ${method} does not exist`);
      }
    }
  }

  private find(id: number): MockMessage {
    const m = this.messages.get(id);
    if (!m) throw notFound(`message ${id} does not exist`);
    return m;
  }

  private mailboxList(accountId: number | undefined): Mailbox[] {
    return this.mailboxes
      .filter((mb) => accountId === undefined || mb.accountId === accountId)
      .map((mb) => {
        let total = 0;
        let unread = 0;
        for (const m of this.messages.values()) {
          if (!m.summary.mailboxIds.includes(mb.id)) continue;
          total++;
          if (!m.summary.flags.seen) unread++;
        }
        return { ...mb, total, unread };
      });
  }

  private get(id: number): Message {
    const m = this.find(id);
    return {
      summary: m.summary,
      to: m.to,
      cc: m.cc,
      replyTo: [],
      messageId: `mock-${id}@mock.test`,
      inReplyTo: "",
      references: [],
      listId: "",
      listUnsubscribe: "",
      parts: m.parts,
      bodyFetched: true,
    };
  }

  private render(id: number, remote: boolean): Rendering {
    const m = this.find(id);
    return { html: m.html, text: m.text, remote: remote ? 0 : m.remote, trackers: m.trackers };
  }

  private part(id: number, path: string) {
    const part = this.find(id).parts.find((pt) => pt.path === path);
    if (!part) throw notFound(`part ${path} of message ${id} does not exist`);
    return {
      path: `m/${id}/${part.filename || `part-${path}`}`,
      contentType: part.contentType,
      filename: part.filename || `part-${path}`,
      size: part.size,
    };
  }

  private thread(id: number): MessageSummary[] {
    const rows = [...this.messages.values()].map((m) => m.summary).filter((s) => s.threadId === id);
    if (rows.length === 0) throw notFound(`thread ${id} does not exist`);
    return rows.sort((a, b) => a.date.localeCompare(b.date) || a.id - b.id);
  }

  private setFlags(list: number[], c: FlagChanges): void {
    this.update(list, (m) => {
      m.summary = { ...m.summary, flags: applyFlags(m.summary.flags, c) };
    });
  }

  private move(list: number[], mailboxId: number): void {
    const target = this.mailboxes.find((mb) => mb.id === mailboxId);
    if (!target) throw new RPCError(ErrorCode.invalidParams, `mailbox ${mailboxId} does not exist`);
    const from = new Set<number>();
    this.update(list, (m) => {
      for (const mb of m.summary.mailboxIds) from.add(mb);
      m.summary = { ...m.summary, mailboxIds: [mailboxId] };
    });
    this.mailboxesChanged(from);
  }

  private delete(list: number[]): void {
    const expunge: number[] = [];
    for (const id of list) {
      const m = this.find(id);
      const trash = this.mailboxes.find((mb) => mb.accountId === m.summary.accountId && mb.role === "trash");
      if (!trash || m.summary.mailboxIds.includes(trash.id)) expunge.push(id);
      else this.move([id], trash.id);
    }
    if (expunge.length > 0) this.remove(expunge);
  }

  private syncNow(accountId: number): void {
    const status = (phase: SyncStatus["phase"]): Event => ({
      event: "sync.progress",
      data: { status: { ...idleStatus(accountId), phase } },
    });
    this.emit(status("syncing"));
    setTimeout(() => this.emit(status("idle")), this.opts.latency ?? 0);
  }

  private openView(query: ViewQuery) {
    const id = this.nextView++;
    const view = { query, ids: this.viewIds(query) };
    this.views.set(id, view);
    return { id, count: view.ids.length };
  }

  private range(id: number, start: number, end: number): MessageSummary[] {
    const view = this.views.get(id);
    if (!view) throw notFound(`view ${id} does not exist`);
    if (start < 0 || end < start) throw new RPCError(ErrorCode.invalidParams, "invalid range");
    return view.ids.slice(start, Math.min(end, view.ids.length)).flatMap((mid) => {
      const m = this.messages.get(mid);
      return m ? [m.summary] : [];
    });
  }

  private viewIds(q: ViewQuery): number[] {
    const role = (mailboxId: number) => this.mailboxes.find((mb) => mb.id === mailboxId)?.role;
    const text = q.text?.trim().toLowerCase() ?? "";
    let rows = [...this.messages.values()]
      .map((m) => m.summary)
      .filter(
        (s) =>
          (q.accountId === undefined || s.accountId === q.accountId) &&
          (q.mailboxId === undefined || s.mailboxIds.includes(q.mailboxId)) &&
          (q.role === undefined || s.mailboxIds.some((mb) => role(mb) === q.role)) &&
          (q.unread === undefined || s.flags.seen !== q.unread) &&
          (q.flagged === undefined || s.flags.flagged === q.flagged) &&
          (text === "" ||
            text
              .split(/\s+/)
              .every((w) => `${s.subject} ${s.from.name} ${s.from.address} ${s.preview}`.toLowerCase().includes(w))),
      )
      .sort(newestFirst);
    if (q.threads) {
      const seen = new Set<number>();
      rows = rows.filter((s) => {
        const key = s.threadId || -s.id;
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
      });
    }
    return rows.map((s) => s.id);
  }

  private refreshViews(): void {
    for (const [id, view] of this.views) {
      const next = this.viewIds(view.query);
      const ops = diffIds(view.ids, next);
      view.ids = next;
      if (ops.length > 0) this.emit({ event: "view.delta", data: { id, count: next.length, ops } });
    }
  }

  private changed(rows: MessageSummary[]): void {
    if (rows.length === 0) return;
    this.refreshViews();
    const byAccount = new Map<number, number[]>();
    const mailboxes = new Set<number>();
    for (const s of rows) {
      byAccount.set(s.accountId, [...(byAccount.get(s.accountId) ?? []), s.id]);
      for (const mb of s.mailboxIds) mailboxes.add(mb);
    }
    for (const [accountId, changedIds] of byAccount) {
      this.emit({ event: "message.changed", data: { accountId, ids: changedIds } });
    }
    this.mailboxesChanged(mailboxes);
  }

  private mailboxesChanged(ids: Set<number>): void {
    for (const id of ids) {
      const mb = this.mailboxes.find((m) => m.id === id);
      if (mb) this.emit({ event: "mailbox.changed", data: { id, accountId: mb.accountId, deleted: false } });
    }
  }
}

/** applyFlags applies FlagChanges with maild's rules for flag colors. */
export function applyFlags(f: Flags, c: FlagChanges): Flags {
  const out = { ...f };
  if (c.seen !== undefined) out.seen = c.seen;
  if (c.answered !== undefined) out.answered = c.answered;
  if (c.flagged !== undefined) {
    out.flagged = c.flagged;
    if (c.flagged && out.flagColor === 0) out.flagColor = 1;
  }
  if (c.flagColor !== undefined) {
    out.flagColor = c.flagColor;
    out.flagged = c.flagColor > 0;
  }
  if (!out.flagged) out.flagColor = 0;
  return out;
}

function newestFirst(a: MessageSummary, b: MessageSummary): number {
  return b.date.localeCompare(a.date) || b.id - a.id;
}

function idleStatus(accountId: number): SyncStatus {
  return { accountId, phase: "idle", done: 0, total: 0 };
}

function notFound(message: string): RPCError {
  return new RPCError(ErrorCode.notFound, message);
}

function num(v: unknown): number {
  if (typeof v !== "number") throw new RPCError(ErrorCode.invalidParams, "expected a number");
  return v;
}

function ids(v: unknown): number[] {
  if (v === undefined || v === null) return [];
  if (!Array.isArray(v) || v.some((x) => typeof x !== "number")) {
    throw new RPCError(ErrorCode.invalidParams, "expected a list of IDs");
  }
  return v as number[];
}
