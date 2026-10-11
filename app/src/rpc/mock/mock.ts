// MockTransport: an in-memory maild for component tests and for running the
// UI in a browser (VITE_MOCK=1). It implements the methods the app uses with
// maild's semantics: views keep ID snapshots and send view.delta after
// changes (an unread view keeps rows that are read), flag, move and copy
// calls emit message.changed and mailbox.changed.
import {
  type Account,
  type AccountCreateParams,
  type AccountUpdateParams,
  type Address,
  type Condition,
  type Conditions,
  type Discovery,
  ErrorCode,
  type Event,
  type FlagChanges,
  type Flags,
  type Mailbox,
  type MailboxRole,
  type Message,
  type MessageSummary,
  type Part,
  type Rendering,
  type RuleApplied,
  type SyncStatus,
  type ViewCount,
  type ViewQuery,
  type ViewSort,
} from "../gen/api";
import { RPCError, type Transport } from "../transport";
import { MockCalendar, type MockCalendarData } from "./calendar";
import { MockCompose, NOT_HANDLED } from "./compose";
import { diffIds } from "./diff";
import { MockOrganize } from "./organize";
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
  private readonly organize: MockOrganize;

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
    this.organize = new MockOrganize(
      (e) => {
        this.emit(e);
        if (e.event === "vip.changed" || e.event === "smart.changed") this.refreshViews();
      },
      (id) => this.people.emailsOf(id),
      (address) => this.people.personOf(address),
      (address) =>
        [...this.messages.values()].find((m) => m.summary.from.address.toLowerCase() === address)?.summary.from.name ??
        "",
    );
    this.organize.unread = (id) =>
      this.viewIds({ smartMailboxId: id }).filter((m) => !this.messages.get(m)?.summary.flags.seen).length;
    this.organize.mailboxExists = (id) => this.mailboxes.some((mb) => mb.id === id);
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
      case "mailbox.create":
        return this.createMailbox(num(p.accountId), String(p.name ?? ""), opt(p.parentId));
      case "mailbox.rename": {
        const mb = this.mailboxOr404(num(p.id));
        const name = this.mailboxName(String(p.name ?? ""), mb.delimiter);
        const at = mb.path.lastIndexOf(mb.delimiter);
        return this.relocate(mb, (mb.delimiter !== "" && at >= 0 ? mb.path.slice(0, at + 1) : "") + name);
      }
      case "mailbox.move": {
        const mb = this.mailboxOr404(num(p.id));
        const parent = opt(p.parentId) === undefined ? undefined : this.mailboxOr404(num(p.parentId));
        if (parent && (parent.id === mb.id || parent.path.startsWith(mb.path + mb.delimiter)))
          throw new RPCError(ErrorCode.conflict, "a mailbox cannot go inside itself");
        return this.relocate(mb, parent ? parent.path + mb.delimiter + mb.name : mb.name);
      }
      case "mailbox.delete":
        this.deleteMailbox(this.mailboxOr404(num(p.id)));
        return null;
      case "mailbox.setRole":
        return this.setRole(this.mailboxOr404(num(p.id)), p.role as MailboxRole);
      case "mailbox.erase":
        return this.erase(this.mailboxOr404(num(p.id)));
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
      case "message.source": {
        const m = this.find(num(p.id));
        const s = m.summary;
        const headers = [
          `From: ${s.from.name ? `${s.from.name} <${s.from.address}>` : s.from.address}`,
          `To: ${m.to.map((a) => a.address).join(", ")}`,
          `Subject: ${s.subject}`,
          `Date: ${new Date(s.date).toUTCString()}`,
          "MIME-Version: 1.0",
          "Content-Type: text/plain; charset=utf-8",
        ].join("\r\n");
        return { headers, text: `${headers}\r\n\r\n${m.text}`, truncated: false };
      }
      case "message.save":
        this.find(num(p.id));
        if (!String(p.path ?? "").startsWith("/"))
          throw new RPCError(ErrorCode.invalidParams, `"${String(p.path)}" is not an absolute path`);
        return null;
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
      case "message.copy":
        this.copy(ids(p.ids), num(p.mailboxId));
        return null;
      case "message.delete":
        this.delete(ids(p.ids));
        return null;
      case "rule.apply":
        return this.applyRules(ids(p.ids));
      case "message.remind":
        this.remind(ids(p.ids), typeof p.at === "string" ? p.at : undefined);
        return null;
      case "thread.messages":
        return this.thread(num(p.id));
      case "view.open":
        return this.openView((p.query ?? {}) as ViewQuery);
      case "view.count":
        return ((p.queries ?? []) as ViewQuery[]).map((q): ViewCount => {
          const ids = this.viewIds({ ...q, threads: false });
          const unread = ids.filter((id) => !this.messages.get(id)?.summary.flags.seen).length;
          return { total: ids.length, unread };
        });
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
        const o = this.organize.dispatch(method, p);
        if (o !== NOT_HANDLED) return o;
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

  /** copy files a new message in the mailbox for each one not there yet,
   *  as a folder server does (maild's Gmail labels are not modeled). */
  private copy(list: number[], mailboxId: number): void {
    const target = this.mailboxes.find((mb) => mb.id === mailboxId);
    if (!target) throw new RPCError(ErrorCode.invalidParams, `mailbox ${mailboxId} does not exist`);
    let next = Math.max(0, ...this.messages.keys()) + 1;
    const copies: MockMessage[] = [];
    for (const id of list) {
      const m = this.find(id);
      if (m.summary.accountId !== target.accountId) {
        throw new RPCError(ErrorCode.invalidParams, `mailbox ${mailboxId} is not in account ${m.summary.accountId}`);
      }
      if (m.summary.mailboxIds.includes(mailboxId)) continue;
      copies.push({ ...m, summary: { ...m.summary, id: next++, mailboxIds: [mailboxId] } });
    }
    if (copies.length > 0) this.add(copies);
  }

  private delete(list: number[]): void {
    // Deleting a message drops its reminder, as in maild.
    for (const id of list) {
      const m = this.messages.get(id);
      if (m?.summary.remindAt !== undefined) {
        const summary = { ...m.summary };
        delete summary.remindAt;
        m.summary = summary;
      }
    }
    const expunge: number[] = [];
    for (const id of list) {
      const m = this.find(id);
      const trash = this.mailboxes.find((mb) => mb.accountId === m.summary.accountId && mb.role === "trash");
      if (!trash || m.summary.mailboxIds.includes(trash.id)) expunge.push(id);
      else this.move([id], trash.id);
    }
    if (expunge.length > 0) this.remove(expunge);
  }

  // Mailbox operations, as maild's (docs/design/organize.md, Mailboxes);
  // the mock has no server, so nothing waits.

  private mailboxOr404(id: number): MockMailbox {
    const mb = this.mailboxes.find((m) => m.id === id);
    if (!mb) throw notFound(`mailbox ${id} does not exist`);
    return mb;
  }

  private writableAccount(id: number): void {
    if (this.account(id).readOnly) throw new RPCError(ErrorCode.conflict, `account ${id} is read-only`);
  }

  private mailboxName(raw: string, delim: string): string {
    const name = raw.trim();
    if (name === "" || [...name].length > 100)
      throw new RPCError(ErrorCode.invalidParams, "a mailbox's name must be 1 to 100 characters");
    if (delim !== "" && name.includes(delim))
      throw new RPCError(ErrorCode.invalidParams, `a mailbox's name cannot hold "${delim}"`);
    return name;
  }

  private taken(accountId: number, path: string): boolean {
    return (
      path.toLowerCase() === "inbox" || this.mailboxes.some((mb) => mb.accountId === accountId && mb.path === path)
    );
  }

  private createMailbox(accountId: number, raw: string, parentId: number | undefined): Mailbox {
    this.writableAccount(accountId);
    const parent = parentId === undefined ? undefined : this.mailboxOr404(parentId);
    if (parent && parent.accountId !== accountId)
      throw new RPCError(ErrorCode.invalidParams, `mailbox ${parent.id} is not in account ${accountId}`);
    const delim = parent?.delimiter || this.mailboxes.find((mb) => mb.accountId === accountId)?.delimiter || "/";
    const name = this.mailboxName(raw, delim);
    const path = parent ? parent.path + delim + name : name;
    if (this.taken(accountId, path)) throw new RPCError(ErrorCode.conflict, `a mailbox "${path}" already exists`);
    const label = this.mailboxes.some((mb) => mb.accountId === accountId && mb.label);
    const mb: MockMailbox = {
      id: Math.max(0, ...this.mailboxes.map((m) => m.id)) + 1,
      accountId,
      path,
      name,
      delimiter: delim,
      role: "none",
      label,
    };
    this.mailboxes.push(mb);
    this.emit({ event: "mailbox.changed", data: { id: mb.id, accountId, deleted: false } });
    return this.mailboxList(accountId).find((m) => m.id === mb.id) as Mailbox;
  }

  private fixedMailbox(mb: MockMailbox): void {
    if (mb.role !== "none" || mb.path.startsWith("[Gmail]"))
      throw new RPCError(ErrorCode.conflict, `mailbox "${mb.path}" is one of the account's own`);
  }

  private relocate(mb: MockMailbox, path: string): Mailbox {
    this.writableAccount(mb.accountId);
    this.fixedMailbox(mb);
    if (path !== mb.path) {
      if (this.taken(mb.accountId, path)) throw new RPCError(ErrorCode.conflict, `a mailbox "${path}" already exists`);
      const old = mb.path;
      for (const o of this.mailboxes) {
        if (o.accountId !== mb.accountId || (o !== mb && !o.path.startsWith(old + o.delimiter))) continue;
        o.path = path + o.path.slice(old.length);
        o.name = o.path.split(o.delimiter).pop() ?? o.path;
        this.emit({ event: "mailbox.changed", data: { id: o.id, accountId: o.accountId, deleted: false } });
      }
    }
    return this.mailboxList(mb.accountId).find((m) => m.id === mb.id) as Mailbox;
  }

  private deleteMailbox(mb: MockMailbox): void {
    this.writableAccount(mb.accountId);
    this.fixedMailbox(mb);
    const gone = this.mailboxes.filter(
      (o) => o.accountId === mb.accountId && (o === mb || o.path.startsWith(mb.path + mb.delimiter)),
    );
    const ids = new Set(gone.map((o) => o.id));
    const orphans: number[] = [];
    const left: number[] = [];
    for (const m of this.messages.values()) {
      if (!m.summary.mailboxIds.some((id) => ids.has(id))) continue;
      const rest = m.summary.mailboxIds.filter((id) => !ids.has(id));
      if (rest.length === 0) orphans.push(m.summary.id);
      else {
        m.summary = { ...m.summary, mailboxIds: rest };
        left.push(m.summary.id);
      }
    }
    for (let i = this.mailboxes.length - 1; i >= 0; i--) {
      if (ids.has(this.mailboxes[i]?.id ?? -1)) this.mailboxes.splice(i, 1);
    }
    if (orphans.length > 0) this.remove(orphans);
    if (left.length > 0) this.changed(left.map((id) => this.find(id).summary));
    for (const id of ids) this.emit({ event: "mailbox.changed", data: { id, accountId: mb.accountId, deleted: true } });
  }

  private setRole(mb: MockMailbox, role: MailboxRole): Mailbox {
    if (!["drafts", "sent", "junk", "trash", "archive"].includes(role))
      throw new RPCError(ErrorCode.invalidParams, `"${role}" is not a role a mailbox can be used for`);
    this.writableAccount(mb.accountId);
    if (mb.label || mb.role === "inbox") throw new RPCError(ErrorCode.conflict, "this mailbox keeps its role");
    for (const o of this.mailboxes) {
      if (o.accountId === mb.accountId && o.role === role && o !== mb) {
        o.role = "none";
        this.emit({ event: "mailbox.changed", data: { id: o.id, accountId: o.accountId, deleted: false } });
      }
    }
    mb.role = role;
    this.emit({ event: "mailbox.changed", data: { id: mb.id, accountId: mb.accountId, deleted: false } });
    return this.mailboxList(mb.accountId).find((m) => m.id === mb.id) as Mailbox;
  }

  private erase(mb: MockMailbox): number {
    if (mb.role !== "trash" && mb.role !== "junk")
      throw new RPCError(ErrorCode.invalidParams, "only a trash or junk mailbox is erased");
    this.writableAccount(mb.accountId);
    const gone = [...this.messages.values()]
      .filter((m) => m.summary.mailboxIds.includes(mb.id))
      .map((m) => m.summary.id);
    if (gone.length > 0) this.remove(gone);
    return gone.length;
  }

  /** remind sets or, without at, clears Remind Me reminders, as maild's
   *  message.remind; the mock never fires them. */
  private remind(list: number[], at: string | undefined): void {
    if (at !== undefined && !(Date.parse(at) > Date.now()))
      throw new RPCError(ErrorCode.invalidParams, "a reminder's time must be in the future");
    const rows = list.map((id) => this.find(id).summary);
    const readOnly = rows.find((s) => this.account(s.accountId).readOnly);
    if (readOnly) throw new RPCError(ErrorCode.conflict, `account ${readOnly.accountId} is read-only`);
    this.update(list, (m) => {
      const summary = { ...m.summary };
      if (at === undefined) delete summary.remindAt;
      else summary.remindAt = at;
      m.summary = summary;
    });
  }

  /** applyRules runs the enabled rules on messages, as maild's rule.apply:
   *  each rule sees the messages as they were, Stop ends a message's run,
   *  and the actions go read marks and flags, copies, then the move or
   *  delete; moves and copies stay within the message's account. */
  private applyRules(list: number[]): RuleApplied {
    const rows = list.map((id) => this.find(id).summary);
    const readOnly = rows.find((s) => this.account(s.accountId).readOnly);
    if (readOnly) throw new RPCError(ErrorCode.conflict, `account ${readOnly.accountId} is read-only`);
    interface Plan {
      read: boolean;
      color: number;
      copies: number[];
      move?: number;
      del: boolean;
    }
    const plans = new Map<number, Plan>();
    let live = rows;
    for (const rule of this.organize.ruleList()) {
      if (!rule.enabled) continue;
      const matched = live.filter((s) => this.matches(s, rule.conditions));
      for (const s of matched) {
        const plan = plans.get(s.id) ?? { read: false, color: 0, copies: [], del: false };
        plans.set(s.id, plan);
        for (const a of rule.actions) {
          if (a.kind === "read") plan.read = true;
          else if (a.kind === "flag" && a.color !== undefined) plan.color = a.color;
          else if (a.kind === "copy" && a.mailboxId !== undefined && !plan.copies.includes(a.mailboxId))
            plan.copies.push(a.mailboxId);
          else if (a.kind === "move" && a.mailboxId !== undefined) plan.move = a.mailboxId;
          else if (a.kind === "delete") plan.del = true;
        }
      }
      if (rule.actions.some((a) => a.kind === "stop")) live = live.filter((s) => !matched.includes(s));
    }
    const sameAccount = (id: number, mailboxId: number) =>
      this.mailboxes.find((mb) => mb.id === mailboxId)?.accountId === this.messages.get(id)?.summary.accountId;
    for (const [id, plan] of plans) {
      if (plan.read) this.setFlags([id], { seen: true });
      if (plan.color !== 0) this.setFlags([id], { flagColor: plan.color });
      for (const mb of plan.copies) if (sameAccount(id, mb)) this.copy([id], mb);
      if (plan.del) this.delete([id]);
      else if (plan.move !== undefined && sameAccount(id, plan.move)) this.move([id], plan.move);
    }
    return { matched: plans.size };
  }

  private syncNow(accountId: number): void {
    const status = (phase: SyncStatus["phase"]): Event => ({
      event: "sync.progress",
      data: { status: { ...idleStatus(accountId), phase } },
    });
    this.emit(status("syncing"));
    setTimeout(() => this.emit(status("idle")), this.opts.latency ?? 0);
  }

  /** inSmart: a smart mailbox lists its conditions' messages, leaving out
   *  those in Trash or Sent unless it includes them. */
  private inSmart(s: MessageSummary, id: number): boolean {
    const smart = this.organize.smart(id);
    if (!smart || !this.matches(s, smart.conditions)) return false;
    const roles = s.mailboxIds.map((mb) => this.mailboxes.find((x) => x.id === mb)?.role);
    if (!smart.includeTrash && roles.includes("trash")) return false;
    return smart.includeSent || !roles.includes("sent");
  }

  private openView(query: ViewQuery) {
    if (query.smartMailboxId !== undefined && !this.organize.smart(query.smartMailboxId))
      throw notFound(`smart mailbox ${query.smartMailboxId} does not exist`);
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

  /** viewIds lists a query's rows; keep's pass the unread condition (rows
   *  of the open view, as maild keeps them). */
  private viewIds(q: ViewQuery, keep: ReadonlySet<number> = new Set()): number[] {
    const role = (mailboxId: number) => this.mailboxes.find((mb) => mb.id === mailboxId)?.role;
    const text = q.text?.trim().toLowerCase() ?? "";
    let rows = [...this.messages.values()]
      .map((m) => m.summary)
      .filter(
        (s) =>
          (q.accountId === undefined || s.accountId === q.accountId) &&
          (q.mailboxId === undefined || s.mailboxIds.includes(q.mailboxId)) &&
          (q.role === undefined || s.mailboxIds.some((mb) => role(mb) === q.role)) &&
          (q.unread === undefined || s.flags.seen !== q.unread || keep.has(s.id)) &&
          (q.flagged === undefined || s.flags.flagged === q.flagged) &&
          (q.hasAttachments === undefined || s.hasAttachments === q.hasAttachments) &&
          (q.conditions === undefined || this.matches(s, q.conditions)) &&
          (q.filter === undefined || this.matches(s, q.filter)) &&
          (q.smartMailboxId === undefined || this.inSmart(s, q.smartMailboxId)) &&
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
    if (q.sort !== undefined && (q.sort !== "date" || q.ascending === true)) {
      const key = (s: MessageSummary): string | number => this.sortKey(s, q.sort ?? "date");
      const dir = q.ascending === true ? 1 : -1;
      rows = [...rows].sort((a, b) => {
        const ka = key(a);
        const kb = key(b);
        return (ka < kb ? -1 : ka > kb ? 1 : 0) * dir || newestFirst(a, b);
      });
    }
    return rows.map((s) => s.id);
  }

  /** sortKey is a message's key for a ViewSort, as maild's. */
  private sortKey(s: MessageSummary, sort: ViewSort): string | number {
    switch (sort) {
      case "from":
        return (s.from.name || s.from.address).toLowerCase();
      case "to": {
        const to = this.messages.get(s.id)?.to[0];
        return (to?.name || to?.address || "").toLowerCase();
      }
      case "subject":
        return s.subject.replace(/^((re|fwd?)\s*:\s*)+/i, "").toLowerCase();
      case "size":
        return s.size;
      case "flags":
        return (s.flags.flagged ? 8 : 0) + s.flags.flagColor;
      case "unread":
        return s.flags.seen ? 0 : 1;
      case "attachments":
        return s.hasAttachments ? 1 : 0;
      default:
        return s.date;
    }
  }

  /** matches evaluates conditions (docs/design/organize.md) for the fields
   *  the app uses; any other field is invalidParams. */
  private matches(s: MessageSummary, c: Conditions): boolean {
    const one = (cond: Condition): boolean => {
      const v = cond.value.trim().toLowerCase();
      const list = v.split(",").map((x) => x.trim());
      const yes = v === "true";
      const text = (...fields: string[]): boolean => {
        const f = fields.map((x) => x.toLowerCase());
        switch (cond.op) {
          case "contains":
            return v.split(/\s+/).every((w) => f.some((x) => x.includes(w)));
          case "notcontains":
            return !v.split(/\s+/).every((w) => f.some((x) => x.includes(w)));
          case "is":
            return f.includes(v);
          case "begins":
            return f.some((x) => x.startsWith(v));
          case "ends":
            return f.some((x) => x.endsWith(v));
          default:
            throw new RPCError(ErrorCode.invalidParams, `${cond.field} does not take ${cond.op}`);
        }
      };
      const among = (value: number | string | undefined, values: readonly (number | string)[]): boolean => {
        const has = values.some((x) => String(x) === String(value));
        return cond.op === "isnot" ? !has : has;
      };
      const role = (id: number) => this.mailboxes.find((mb) => mb.id === id)?.role;
      switch (cond.field) {
        case "from":
          return text(s.from.name, s.from.address);
        case "subject":
          return text(s.subject);
        case "content":
          return text(`${s.subject} ${s.from.name} ${s.from.address} ${s.preview}`);
        case "tome":
        case "ccme": {
          const m = this.messages.get(s.id);
          const me = new Set(this.accounts.map((a) => a.email.toLowerCase()));
          const list = cond.field === "tome" ? (m?.to ?? []) : (m?.cc ?? []);
          return list.some((a) => me.has(a.address.toLowerCase())) === yes;
        }
        case "color":
          return s.flags.flagged ? among(s.flags.flagColor, list) : cond.op === "isnot";
        case "vip":
          return this.organize.isVip(s.from.address) === yes;
        case "flagged":
          return s.flags.flagged === yes;
        case "unread":
          return !s.flags.seen === yes;
        case "attachments":
          return s.hasAttachments === yes;
        case "account":
          return among(s.accountId, list);
        case "mailbox":
          return cond.op === "isnot"
            ? !s.mailboxIds.some((id) => list.includes(String(id)))
            : s.mailboxIds.some((id) => list.includes(String(id)));
        case "role":
          return cond.op === "isnot"
            ? !s.mailboxIds.some((id) => list.includes(String(role(id))))
            : s.mailboxIds.some((id) => list.includes(String(role(id))));
        default:
          throw new RPCError(ErrorCode.invalidParams, `the mock does not compile ${cond.field}`);
      }
    };
    if (c.conditions.length === 0) return true;
    return c.match === "any" ? c.conditions.some(one) : c.conditions.every(one);
  }

  private refreshViews(): void {
    for (const [id, view] of this.views) {
      const next = this.viewIds(view.query, view.query.unread === undefined ? undefined : new Set(view.ids));
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

function opt(v: unknown): number | undefined {
  return v === undefined || v === null ? undefined : num(v);
}

function ids(v: unknown): number[] {
  if (v === undefined || v === null) return [];
  if (!Array.isArray(v) || v.some((x) => typeof x !== "number")) {
    throw new RPCError(ErrorCode.invalidParams, "expected a list of IDs");
  }
  return v as number[];
}
