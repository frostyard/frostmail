// The people domain and account collections of MockTransport: address
// books and the people in them, with maild's matching, ordering and contact
// cards (docs/specs/pim-ui.md; internal/engine/people.go).
import {
  type Account,
  type Address,
  type Collection,
  type Contact,
  type ContactCard,
  ErrorCode,
  type Event,
  type MessageSummary,
  type Person,
  type PersonSummary,
  type Photo,
} from "../gen/api";
import { RPCError } from "../transport";
import { NOT_HANDLED } from "./compose";

/** MockPeopleData is the address books and people the mock serves. */
export interface MockPeopleData {
  collections: Collection[];
  people: Person[];
  /** Photos by person ID. */
  photos?: Record<number, Photo>;
}

/** MockMail is what contact cards need from the stored mail. */
export interface MockMail {
  summary: MessageSummary;
  to: Address[];
  cc: Address[];
}

type Params = Record<string, unknown>;

const words = (s: string): string[] =>
  s
    .toLowerCase()
    .split(/[^\p{L}\p{N}]+/u)
    .filter((w) => w !== "");

/** sortKey files a person by the first contact's family name, else the name. */
function sortKey(p: Person): string {
  const c = p.contacts[0];
  const family = c?.familyName.trim() ?? "";
  return (family !== "" ? `${family} ${c?.givenName ?? ""}` : p.displayName).trim().toLowerCase();
}

function firstEmail(p: Person): string {
  for (const c of p.contacts) {
    const e = c.emails[0];
    if (e) return e.value.toLowerCase();
  }
  return "";
}

/** MockPeople answers account.collections and the people domain. */
export class MockPeople {
  private nextId: number;

  constructor(
    private readonly data: MockPeopleData,
    private readonly emit: (e: Event) => void,
    private readonly mail: () => MockMail[],
    private readonly accounts: () => Account[],
  ) {
    this.nextId = 1 + Math.max(1000, ...data.people.flatMap((p) => [p.id, ...p.contacts.map((c) => c.id)]));
  }

  /** dispatch answers a method of these domains, or returns NOT_HANDLED. */
  dispatch(method: string, p: Params): unknown {
    switch (method) {
      case "account.collections":
        return this.data.collections.filter(
          (c) =>
            (typeof p.accountId !== "number" || c.accountId === p.accountId) &&
            (typeof p.kind !== "string" || c.kind === p.kind),
        );
      case "people.list":
        return this.list(
          typeof p.query === "string" ? p.query : "",
          typeof p.collectionId === "number" ? p.collectionId : undefined,
        );
      case "people.get":
        return this.person(Number(p.id));
      case "people.card":
        return this.card(String(p.email ?? ""));
      case "people.photo": {
        const photo = this.data.photos?.[Number(p.id)];
        if (!photo) throw new RPCError(ErrorCode.notFound, `person photo ${Number(p.id)} does not exist`);
        return photo;
      }
      case "people.add":
        return this.add(String(p.email ?? ""), typeof p.name === "string" ? p.name : "");
      default:
        return NOT_HANDLED;
    }
  }

  private sorted(): Person[] {
    return [...this.data.people].sort((a, b) => sortKey(a).localeCompare(sortKey(b)) || a.id - b.id);
  }

  private list(query: string, collectionId: number | undefined): PersonSummary[] {
    const q = words(query);
    return this.sorted()
      .filter((p) => collectionId === undefined || p.contacts.some((c) => c.collectionId === collectionId))
      .filter((p) => {
        const ws = [p.displayName, p.organization, ...p.contacts.flatMap((c) => c.emails.map((e) => e.value))].flatMap(
          words,
        );
        return q.every((w) => ws.some((x) => x.startsWith(w)));
      })
      .map((p) => {
        const letter = sortKey(p).charAt(0).toUpperCase();
        return {
          id: p.id,
          displayName: p.displayName,
          organization: p.organization,
          email: firstEmail(p),
          hasPhoto: p.hasPhoto,
          index: /\p{L}/u.test(letter) ? letter : "#",
        };
      });
  }

  private person(id: number): Person {
    const p = this.data.people.find((x) => x.id === id);
    if (!p) throw new RPCError(ErrorCode.notFound, `person ${id} does not exist`);
    return structuredClone(p);
  }

  private byEmail(email: string): Person | undefined {
    return this.data.people.find((p) => p.contacts.some((c) => c.emails.some((e) => e.value.toLowerCase() === email)));
  }

  private card(raw: string): ContactCard {
    const email = raw.trim().toLowerCase();
    if (!email.includes("@")) throw new RPCError(ErrorCode.invalidParams, "email must contain @");
    const person = this.byEmail(email);
    const mine = (a: Address) => a.address.toLowerCase() === email;
    const recent = this.mail()
      .filter((m) => mine(m.summary.from) || m.to.some(mine) || m.cc.some(mine))
      .map((m) => m.summary)
      .sort((a, b) => b.date.localeCompare(a.date))
      .slice(0, 5);
    const seen = this.mail()
      .flatMap((m) => [m.summary.from, ...m.to, ...m.cc])
      .find(mine);
    return {
      email,
      name: person?.displayName ?? seen?.name ?? "",
      person: person ? structuredClone(person) : undefined,
      recent,
      upcoming: [],
      canAdd: !person && this.writableBooks().length > 0,
    };
  }

  private writableBooks(): Collection[] {
    const readOnly = new Set(
      this.accounts()
        .filter((a) => a.readOnly)
        .map((a) => a.id),
    );
    return this.data.collections.filter(
      (c) => c.kind === "addressbook" && c.enabled && !c.readOnly && !readOnly.has(c.accountId),
    );
  }

  private add(raw: string, name: string): Person {
    const email = raw.trim().toLowerCase();
    if (!email.includes("@")) throw new RPCError(ErrorCode.invalidParams, "email must contain @");
    if (this.byEmail(email)) throw new RPCError(ErrorCode.conflict, `a contact for ${email} already exists`);
    const book = this.writableBooks()[0];
    if (!book) throw new RPCError(ErrorCode.notFound, "no address book can take a contact");
    const id = this.nextId++;
    const parts = name
      .trim()
      .split(/\s+/)
      .filter((w) => w !== "");
    const contact: Contact = {
      id,
      collectionId: book.id,
      accountId: book.accountId,
      displayName: name.trim() || email,
      givenName: parts.length > 1 ? parts.slice(0, -1).join(" ") : (parts[0] ?? ""),
      familyName: parts.length > 1 ? (parts[parts.length - 1] ?? "") : "",
      nickname: "",
      organization: "",
      title: "",
      emails: [{ label: "", value: email }],
      phones: [],
      addresses: [],
      urls: [],
      birthday: "",
      note: "",
      readOnly: false,
    };
    const person: Person = {
      id,
      displayName: contact.displayName,
      organization: "",
      hasPhoto: false,
      contacts: [contact],
    };
    this.data.people.push(person);
    this.emit({ event: "people.changed", data: { accountId: book.accountId } });
    return structuredClone(person);
  }
}
