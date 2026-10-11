// The settings, VIP, smart mailbox and rule domains of MockTransport, as
// maild keeps them (docs/design/organize.md; internal/engine/organize.go).
// rule.apply is MockTransport's, which holds the messages.
import {
  type Conditions,
  ErrorCode,
  type Event,
  type NotifyScope,
  type Rule,
  type RuleAction,
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

const KINDS: readonly RuleAction["kind"][] = ["move", "copy", "read", "flag", "delete", "notify", "stop"];

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
  /** Rules in position order. */
  private rules: Rule[] = [];
  private nextRule = 1;
  /** mailboxExists reports whether a mailbox exists (MockTransport's). */
  mailboxExists: (id: number) => boolean = () => true;

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

  /** ruleList is every rule, in the order they run. */
  ruleList(): Rule[] {
    return structuredClone(this.rules);
  }

  /** isVip reports whether an address is a VIP. */
  isVip(address: string): boolean {
    return this.vips.has(address.trim().toLowerCase());
  }

  dispatch(method: string, p: Params): unknown {
    switch (method) {
      case "settings.get":
        return this.shown(this.settings);
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
      case "rule.list":
        return this.rules.map((r) => this.checked(r));
      case "rule.create": {
        const r: Rule = {
          id: this.nextRule++,
          name: ruleName(p.name),
          position: this.rules.length,
          enabled: p.enabled !== false,
          conditions: p.conditions as Conditions,
          actions: this.actions(p.actions),
        };
        this.rules.push(r);
        this.emit({ event: "rule.changed", data: { id: r.id, deleted: false } });
        return this.checked(r);
      }
      case "rule.update": {
        const r = this.ruleOr404(Number(p.id));
        // Only what changes is checked, as maild does.
        const name = p.name !== undefined ? ruleName(p.name) : r.name;
        const actions = p.actions !== undefined ? this.actions(p.actions) : r.actions;
        r.name = name;
        r.actions = actions;
        if (p.conditions !== undefined) r.conditions = p.conditions as Conditions;
        if (p.enabled !== undefined) r.enabled = p.enabled === true;
        this.emit({ event: "rule.changed", data: { id: r.id, deleted: false } });
        return this.checked(r);
      }
      case "rule.delete": {
        const r = this.ruleOr404(Number(p.id));
        this.rules = this.rules.filter((x) => x !== r);
        this.rules.forEach((x, i) => {
          x.position = i;
        });
        this.emit({ event: "rule.changed", data: { id: r.id, deleted: true } });
        return null;
      }
      case "rule.move": {
        const r = this.ruleOr404(Number(p.id));
        if (Number(p.position) < 0) throw invalid("position must not be negative");
        const rest = this.rules.filter((x) => x !== r);
        rest.splice(Math.min(Number(p.position), rest.length), 0, r);
        this.rules = rest;
        this.rules.forEach((x, i) => {
          x.position = i;
        });
        this.emit({ event: "rule.changed", data: { id: r.id, deleted: false } });
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

  private ruleOr404(id: number): Rule {
    const r = this.rules.find((x) => x.id === id);
    if (!r) throw new RPCError(ErrorCode.notFound, `rule ${id} does not exist`);
    return r;
  }

  /** checked is a rule with the problem of an action whose mailbox is gone. */
  private checked(r: Rule): Rule {
    const out = structuredClone(r);
    delete out.problem;
    const gone = r.actions.findIndex((a) => a.mailboxId !== undefined && !this.mailboxExists(a.mailboxId));
    if (gone >= 0) out.problem = `action ${gone + 1}: mailbox ${r.actions[gone]?.mailboxId} is gone`;
    return out;
  }

  /** actions checks a rule's actions as maild's checkActions does. */
  private actions(raw: unknown): RuleAction[] {
    const actions = (Array.isArray(raw) ? raw : []) as RuleAction[];
    if (actions.length === 0 || actions.length > 20) throw invalid("a rule needs 1 to 20 actions");
    actions.forEach((a, i) => {
      const n = i + 1;
      if (!KINDS.includes(a.kind)) throw invalid(`action ${n}: "${String(a.kind)}" is not an action`);
      const needsMailbox = a.kind === "move" || a.kind === "copy";
      if (needsMailbox && a.mailboxId === undefined) throw invalid(`action ${n}: ${a.kind} needs a mailboxId`);
      if (needsMailbox && a.mailboxId !== undefined && !this.mailboxExists(a.mailboxId))
        throw new RPCError(ErrorCode.notFound, `action ${n}: mailbox ${a.mailboxId} does not exist`);
      if (!needsMailbox && a.mailboxId !== undefined) throw invalid(`action ${n}: ${a.kind} takes no mailboxId`);
      if (a.kind === "flag" && (a.color === undefined || a.color < 1 || a.color > 7))
        throw invalid(`action ${n}: flag needs a color 1-7`);
      if (a.kind !== "flag" && a.color !== undefined) throw invalid(`action ${n}: ${a.kind} takes no color`);
    });
    return structuredClone(actions);
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
    if (p.favorites !== undefined) {
      const favorites = (p.favorites as number[]).map(Number);
      if (favorites.length > 50 || new Set(favorites).size !== favorites.length)
        throw invalid("favorites must be at most 50 mailboxes, without repeats");
      const gone = favorites.find((id) => !this.mailboxExists(id));
      if (gone !== undefined) throw new RPCError(ErrorCode.notFound, `mailbox ${gone} does not exist`);
      next.favorites = favorites;
    }
    this.settings = next;
    this.emit({ event: "settings.changed", data: {} });
    return this.shown(next);
  }

  /** shown is the settings as maild gives them: deleted favorites left
   *  out, and none at all absent. */
  private shown(s: Settings): Settings {
    const out = structuredClone(s);
    const favorites = (out.favorites ?? []).filter((id) => this.mailboxExists(id));
    if (favorites.length > 0) out.favorites = favorites;
    else delete out.favorites;
    return out;
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

function ruleName(raw: unknown): string {
  const name = String(raw ?? "").trim();
  if (name === "" || [...name].length > 100) throw invalid("a rule's name must be 1 to 100 characters");
  return name;
}

function smartName(raw: unknown): string {
  const name = String(raw ?? "").trim();
  if (name === "" || [...name].length > 100) throw invalid("a smart mailbox's name must be 1 to 100 characters");
  return name;
}
