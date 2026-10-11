// The settings and VIP domains of MockTransport, as maild keeps them
// (docs/design/organize.md; internal/engine/organize.go).
import { ErrorCode, type Event, type NotifyScope, type Settings, type Vip } from "../gen/api";
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

  constructor(
    private readonly emit: (e: Event) => void,
    private readonly person: MockPersonEmails,
    private readonly personOf: (address: string) => { id: number; name: string } | undefined,
    private readonly seenName: (address: string) => string,
  ) {}

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
    }
    return NOT_HANDLED;
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
      if (scope === "smart") throw new RPCError(ErrorCode.unavailable, "not implemented yet: M5 phase 3");
      next.notifyScope = scope;
      delete next.notifySmartId;
    }
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
