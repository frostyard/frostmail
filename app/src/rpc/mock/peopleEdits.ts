// Contact writes and single-use restores for MockPeople (ADR-0030, ADR-0032).
import {
  type Account,
  type Collection,
  type Contact,
  type ContactFields,
  type ContactWrite,
  ErrorCode,
  type Event,
  type Person,
  type Photo,
} from "../gen/api";
import { RPCError } from "../transport";
import type { MockPeopleData } from "./people";

interface Restore {
  contactId: number;
  collectionId: number;
  accountId: number;
  personId: number;
  position: number;
  contact?: Contact;
  photo?: Photo;
}

function derivedName(c: Contact): string {
  return `${c.givenName} ${c.familyName}`.trim() || c.organization.trim() || c.emails[0]?.value || "";
}

function validBirthday(value: string): boolean {
  if (value === "") return true;
  if (!/^(?:\d{4}|-)-\d{2}-\d{2}$/.test(value)) return false;
  const full = value.startsWith("--") ? `2000${value.slice(1)}` : value;
  const date = new Date(`${full}T00:00:00Z`);
  return !Number.isNaN(date.getTime()) && date.toISOString().slice(0, 10) === full;
}

function patch(contact: Contact, fields: ContactFields): Contact {
  const changed: Contact = {
    ...contact,
    ...structuredClone(Object.fromEntries(Object.entries(fields).filter(([, value]) => value !== undefined))),
  };
  if (changed.emails.some((email) => !/^[^\s@]+@[^\s@]+$/.test(email.value))) {
    throw new RPCError(ErrorCode.invalidParams, "email must contain text on both sides of @ and no spaces");
  }
  if (!validBirthday(changed.birthday)) throw new RPCError(ErrorCode.invalidParams, "invalid birthday");
  if (contact.displayName === "" || contact.displayName === derivedName(contact)) {
    changed.displayName = derivedName(changed);
  }
  return changed;
}

function validatePhoto(photo: Photo): void {
  if (photo.contentType !== "image/jpeg" && photo.contentType !== "image/png") {
    throw new RPCError(ErrorCode.invalidParams, "photo must be JPEG or PNG");
  }
  let size: number;
  try {
    size = atob(photo.data).length;
  } catch {
    throw new RPCError(ErrorCode.invalidParams, "photo must be base64-encoded");
  }
  if (size > 256 * 1024) throw new RPCError(ErrorCode.invalidParams, "photo must be at most 256 KB");
}

/** MockContactEdits applies contact patches and keeps what each write replaced. */
export class MockContactEdits {
  private nextRestoreId = 1;
  private readonly restores = new Map<string, Restore>();

  constructor(
    private readonly data: MockPeopleData,
    private readonly emit: (e: Event) => void,
    private readonly accounts: () => Account[],
    private readonly nextId: () => number,
    private readonly writableBooks: () => Collection[],
  ) {}

  /** create adds a contact in its own person, using the first writable book by default. */
  create(fields: ContactFields, collectionId?: number): ContactWrite {
    const book = collectionId === undefined ? this.writableBooks()[0] : this.book(collectionId);
    if (!book) throw new RPCError(ErrorCode.notFound, "no address book can take a contact");
    const contact = patch(
      {
        id: 0,
        collectionId: book.id,
        accountId: book.accountId,
        displayName: "",
        givenName: "",
        familyName: "",
        nickname: "",
        organization: "",
        title: "",
        emails: [],
        phones: [],
        addresses: [],
        urls: [],
        birthday: "",
        note: "",
        readOnly: false,
      },
      fields,
    );
    if (!contact.displayName)
      throw new RPCError(ErrorCode.invalidParams, "contact needs a name, organization or email");
    contact.id = this.nextId();
    const person: Person = {
      id: contact.id,
      displayName: contact.displayName,
      organization: contact.organization,
      hasPhoto: false,
      contacts: [contact],
    };
    const restoreId = this.keep(person, contact, true);
    this.data.people.push(person);
    return this.written(person, contact, restoreId);
  }

  /** update replaces the supplied fields, preserving a custom display name. */
  update(id: number, fields: ContactFields): ContactWrite {
    const { person, contact, position } = this.writableContact(id);
    const changed = patch(contact, fields);
    const restoreId = this.keep(person, contact);
    person.contacts[position] = changed;
    return this.written(person, changed, restoreId);
  }

  /** delete removes a contact and, for the last contact, its person and photo. */
  delete(id: number): ContactWrite {
    const { person, contact, position } = this.writableContact(id);
    const restoreId = this.keep(person, contact);
    person.contacts.splice(position, 1);
    return this.written(person, contact, restoreId);
  }

  /** setPhoto stores a validated photo under the person ID, or removes it. */
  setPhoto(id: number, photo?: Photo): ContactWrite {
    const { person, contact } = this.writableContact(id);
    if (photo) validatePhoto(photo);
    const restoreId = this.keep(person, contact);
    this.putPhoto(person.id, photo);
    return this.written(person, contact, restoreId);
  }

  /** restore writes back the saved contact and photo without replacing other contacts. */
  restore(restoreId: string): null {
    const saved = this.restores.get(restoreId);
    if (!saved) throw new RPCError(ErrorCode.notFound, `restore ${restoreId} does not exist`);
    this.book(saved.collectionId);
    let person = this.data.people.find((p) => p.id === saved.personId);
    const current = person?.contacts.find((c) => c.id === saved.contactId);
    if (current) this.writableContact(current.id);
    if (saved.contact) {
      if (!person) {
        person = { id: saved.personId, displayName: "", organization: "", hasPhoto: false, contacts: [] };
        this.data.people.push(person);
      }
      const position = person.contacts.findIndex((c) => c.id === saved.contactId);
      if (position >= 0) person.contacts.splice(position, 1);
      person.contacts.splice(saved.position, 0, structuredClone(saved.contact));
    } else if (person) {
      person.contacts = person.contacts.filter((c) => c.id !== saved.contactId);
    }
    this.putPhoto(saved.personId, saved.photo);
    if (person) this.refresh(person);
    this.restores.delete(restoreId);
    this.emit({ event: "people.changed", data: { accountId: saved.accountId } });
    return null;
  }

  private book(id: number): Collection {
    const book = this.data.collections.find((c) => c.id === id && c.kind === "addressbook");
    if (!book) throw new RPCError(ErrorCode.notFound, `address book ${id} does not exist`);
    if (!this.writableBooks().some((c) => c.id === id)) {
      throw new RPCError(ErrorCode.conflict, `address book ${id} is not writable`);
    }
    return book;
  }

  private writableContact(id: number): { person: Person; contact: Contact; position: number } {
    for (const person of this.data.people) {
      const position = person.contacts.findIndex((c) => c.id === id);
      const contact = person.contacts[position];
      if (!contact) continue;
      if (contact.readOnly || this.accounts().some((a) => a.id === contact.accountId && a.readOnly)) {
        throw new RPCError(ErrorCode.conflict, `contact ${id} is not writable`);
      }
      this.book(contact.collectionId);
      return { person, contact, position };
    }
    throw new RPCError(ErrorCode.notFound, `contact ${id} does not exist`);
  }

  private keep(person: Person, contact: Contact, created = false): string {
    const restoreId = `r${this.nextRestoreId++}`;
    this.restores.set(
      restoreId,
      structuredClone({
        contactId: contact.id,
        collectionId: contact.collectionId,
        accountId: contact.accountId,
        personId: person.id,
        position: person.contacts.indexOf(contact),
        contact: created ? undefined : contact,
        photo: this.data.photos?.[person.id],
      }),
    );
    return restoreId;
  }

  private putPhoto(personId: number, photo?: Photo): void {
    if (photo) {
      this.data.photos ??= {};
      this.data.photos[personId] = structuredClone(photo);
    } else if (this.data.photos) {
      delete this.data.photos[personId];
    }
  }

  private refresh(person: Person): Person | undefined {
    const first = person.contacts[0];
    if (!first) {
      this.data.people = this.data.people.filter((p) => p.id !== person.id);
      this.putPhoto(person.id);
      return undefined;
    }
    person.displayName = first.displayName;
    person.organization = first.organization;
    person.hasPhoto = Boolean(this.data.photos?.[person.id]);
    return person;
  }

  private written(person: Person, contact: Contact, restoreId: string): ContactWrite {
    const result = structuredClone({ contactId: contact.id, person: this.refresh(person), restoreId });
    this.emit({ event: "people.changed", data: { accountId: contact.accountId } });
    return result;
  }
}
