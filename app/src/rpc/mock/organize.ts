// The settings and VIP domains of MockTransport, as maild keeps them
// (docs/design/organize.md; internal/engine/organize.go).
import {
  type Conditions,
  ErrorCode,
  type Event,
  type NotifyScope,
  type Settings,
  type SmartMailbox,
  type Vip,
} from "../gen/api";
import { RPCError } from "../transport";
import { NOT_HANDLED } from "./compose";

type Params = Record<string, unknown>;

/** MockPersonEmails names a person and their addresses, lowercased. */
export type MockPersonEmails = (personId: number) => { name: string; emails: string[] } | undefined;

const SCOPES: readonly NotifyScope[] = ["inbox", "vips", "contacts", "all", "smart"];

const invalid = (message: string) => new RPCError(ErrorCode.invalidParams, message);

/** MockOrganize keeps settings and VIPs. */
export class MockOrganize {
  private settings: Settings = { undoDelay: 10, notifyScope: "inbox", flagNames: ["", "", "", "", "", "", ""] };
  /** VIP addresses, lowercased, with the name seen when added. */
  private readonly vips = new Map<string, string>();
  /** Smart mailboxes in position order; unread is counted when listed. */
  private smarts: SmartMailbox[] = [];
  private nextSmart = 1;
  /** unread counts a smart mailbox's unread messages (MockTransport's views). */
  unread: (id: number) => number = () => 0;

  constructor(
    private readonly emit: (e: Event) => void,
    private readonly person: MockPersonEmails,
    private readonly personOf: (address: string) => { id: number; name: string } | undefined,
    private readonly seenName: (address: string) => string,
  ) {}

  /** smart is a smart mailbox, for views of it. */
  smart(id: number): SmartMailbox | undefined {
    return this.smarts.find((s) => s.id === id);
  }

  /** isVip reports whether an address is a VIP. */
  isVip(address: string): boolean {
    return this.vips.has(address.trim().toLowerCase());
  }

  dispatch(method: string, p: Params): unknown {
    switch (method) {
      case "settings.get":
        return structuredClone(this.settings);
      case "settings.set":
        return this.set(p);
      case "vip.list":
        return this.list();
      case "vip.add":
        for (const a of this.addresses(p)) {
          if (!this.vips.has(a)) this.vips.set(a, this.seenName(a));
        }
        this.emit({ event: "vip.changed", data: {} });
        return this.list();
      case "vip.remove":
        for (const a of this.addresses(p)) this.vips.delete(a);
        this.emit({ event: "vip.changed", data: {} });
        return null;
      case "smart.list":
        return this.smarts.map((s) => this.counted(s));
      case "smart.create": {
        const s: SmartMailbox = {
          id: this.nextSmart++,
          name: smartName(p.name),
          position: this.smarts.length,
          conditions: p.conditions as Conditions,
          includeTrash: p.includeTrash === true,
          includeSent: p.includeSent === true,
          unread: 0,
        };
        this.smarts.push(s);
        this.emit({ event: "smart.changed", data: { id: s.id, deleted: false } });
        return this.counted(s);
      }
      case "smart.update": {
        const s = this.smartOr404(Number(p.id));
        if (p.name !== undefined) s.name = smartName(p.name);
        if (p.conditions !== undefined) s.conditions = p.conditions as Conditions;
        if (p.includeTrash !== undefined) s.includeTrash = p.includeTrash === true;
        if (p.includeSent !== undefined) s.includeSent = p.includeSent === true;
        this.emit({ event: "smart.changed", data: { id: s.id, deleted: false } });
        return this.counted(s);
      }
      case "smart.delete": {
        const s = this.smartOr404(Number(p.id));
        this.smarts = this.smarts.filter((x) => x !== s);
        this.smarts.forEach((x, i) => {
          x.position = i;
        });
        if (this.settings.notifyScope === "smart" && this.settings.notifySmartId === s.id) {
          this.settings = { ...this.settings, notifyScope: "inbox" };
          delete this.settings.notifySmartId;
          this.emit({ event: "settings.changed", data: {} });
        }
        this.emit({ event: "smart.changed", data: { id: s.id, deleted: true } });
        return null;
      }
      case "smart.move": {
        const s = this.smartOr404(Number(p.id));
        const rest = this.smarts.filter((x) => x !== s);
        rest.splice(Math.min(Math.max(Number(p.position), 0), rest.length), 0, s);
        this.smarts = rest;
        this.smarts.forEach((x, i) => {
          x.position = i;
        });
        this.emit({ event: "smart.changed", data: { id: s.id, deleted: false } });
        return null;
      }
      case "smart.fromSearch": {
        // A rough stand-in for maild's search.ToConditions: each word of the
        // text is "content contains" it.
        const words = String(p.text ?? "")
          .split(/\s+/)
          .filter((w) => w !== "");
        const conditions: Conditions = {
          match: "all",
          conditions: words.map((value) => ({ field: "content", op: "contains", value })),
        };
        if (p.mailboxId !== undefined)
          conditions.conditions.push({ field: "mailbox", op: "is", value: String(p.mailboxId) });
        return conditions;
      }
    }
    return NOT_HANDLED;
  }

  private smartOr404(id: number): SmartMailbox {
    const s = this.smart(id);
    if (!s) throw new RPCError(ErrorCode.notFound, `smart mailbox ${id} does not exist`);
    return s;
  }

  private counted(s: SmartMailbox): SmartMailbox {
    return structuredClone({ ...s, unread: this.unread(s.id) });
  }

  private set(p: Params): Settings {
    const next = structuredClone(this.settings);
    if (p.undoDelay !== undefined) {
      if (![0, 10, 20, 30].includes(Number(p.undoDelay))) throw invalid("undoDelay must be 0, 10, 20 or 30 seconds");
      next.undoDelay = Number(p.undoDelay);
    }
    if (p.notifyScope !== undefined) {
      const scope = p.notifyScope as NotifyScope;
      if (!SCOPES.includes(scope)) throw invalid(`notifyScope "${String(scope)}" is unknown`);
      next.notifyScope = scope;
      delete next.notifySmartId;
    }
    if (p.notifySmartId !== undefined) {
      if (next.notifyScope !== "smart") throw invalid("notifySmartId goes with notifyScope smart");
      if (!this.smart(Number(p.notifySmartId)))
        throw new RPCError(ErrorCode.notFound, `smart mailbox ${Number(p.notifySmartId)} does not exist`);
      next.notifySmartId = Number(p.notifySmartId);
    }
    if (next.notifyScope === "smart" && next.notifySmartId === undefined)
      throw invalid("notifyScope smart needs notifySmartId");
    if (p.flagNames !== undefined) {
      const names = p.flagNames as string[];
      if (!Array.isArray(names) || names.length !== 7) throw invalid("flagNames needs 7 names, one per color");
      next.flagNames = names.map((n) => String(n).trim());
      if (next.flagNames.some((n) => [...n].length > 40)) throw invalid("a flag name is longer than 40 characters");
    }
    this.settings = next;
    this.emit({ event: "settings.changed", data: {} });
    return structuredClone(next);
  }

  private addresses(p: Params): string[] {
    const given = (p.addresses as string[] | undefined) ?? [];
    if (given.length === 0 && p.personId === undefined) throw invalid("give addresses or a personId");
    const out = given.map((a) => String(a).trim().toLowerCase());
    if (out.some((a) => !a.includes("@") || /\s/.test(a))) throw invalid("not an email address");
    if (p.personId !== undefined) {
      const person = this.person(Number(p.personId));
      if (!person) throw new RPCError(ErrorCode.notFound, `person ${Number(p.personId)} does not exist`);
      out.push(...person.emails);
    }
    return out;
  }

  private list(): Vip[] {
    return [...this.vips]
      .map(([address, seen]): Vip => {
        const person = this.personOf(address);
        return person ? { address, name: person.name, personId: person.id } : { address, name: seen };
      })
      .sort((a, b) => a.name.toLowerCase().localeCompare(b.name.toLowerCase()) || a.address.localeCompare(b.address));
  }
}

function smartName(raw: unknown): string {
  const name = String(raw ?? "").trim();
  if (name === "" || [...name].length > 100) throw invalid("a smart mailbox's name must be 1 to 100 characters");
  return name;
}
