import type { Contact, ContactFields, LabeledValue, PostalAddress } from "../rpc/gen/api";

/** ListKind identifies a contact's labeled lists. */
export type ListKind = "email" | "phone" | "address" | "url";

/** LABELS lists each kind's standard labels in menu order. */
export const LABELS = {
  email: ["home", "work", "other"],
  phone: ["mobile", "home", "work", "main", "fax", "pager", "other"],
  address: ["home", "work", "other"],
  url: ["homepage", "home", "work", "other"],
} as const;

/** Birthday keeps the editable date parts, including an optional year. */
export interface Birthday {
  month: string;
  day: string;
  year: string;
}

/** ContactDraft contains only the contact's editable fields. */
export interface ContactDraft {
  givenName: string;
  familyName: string;
  nickname: string;
  organization: string;
  title: string;
  emails: LabeledValue[];
  phones: LabeledValue[];
  addresses: PostalAddress[];
  urls: LabeledValue[];
  birthday: Birthday;
  note: string;
}

/** DraftErrors associates validation messages with their fields. */
export type DraftErrors = Record<string, string>;

/** draftOf makes an independent editable copy, or an empty new contact. */
export function draftOf(contact: Contact | null): ContactDraft {
  const birthday = contact?.birthday ?? "";
  const [year = "", month = "", day = ""] = birthday.startsWith("--")
    ? ["", ...birthday.slice(2).split("-")]
    : birthday.split("-");
  return {
    givenName: contact?.givenName ?? "",
    familyName: contact?.familyName ?? "",
    nickname: contact?.nickname ?? "",
    organization: contact?.organization ?? "",
    title: contact?.title ?? "",
    emails: contact?.emails.map((row) => ({ ...row })) ?? [],
    phones: contact?.phones.map((row) => ({ ...row })) ?? [],
    addresses: contact?.addresses.map((row) => ({ ...row })) ?? [],
    urls: contact?.urls.map((row) => ({ ...row })) ?? [],
    birthday: { month, day, year },
    note: contact?.note ?? "",
  };
}

/** birthdayValue writes a birthday in the API's date format. */
export function birthdayValue({ month, day, year }: Birthday): string {
  if (!month && !day && !year) return "";
  return year ? `${year}-${month}-${day}` : `--${month}-${day}`;
}

/** nextLabel picks the first unused standard label, falling back to the last. */
export function nextLabel(kind: ListKind, used: readonly string[]): string {
  const labels = LABELS[kind];
  return labels.find((label) => !used.includes(label)) ?? labels[labels.length - 1] ?? "other";
}

function validBirthday({ month, day, year }: Birthday): boolean {
  if (!month && !day && !year) return true;
  if (!/^\d{2}$/.test(month) || !/^\d{2}$/.test(day) || (year && !/^\d{4}$/.test(year))) return false;
  const y = Number(year || "2000");
  const leap = y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0);
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  return Number(day) >= 1 && Number(day) <= (days[Number(month) - 1] ?? 0);
}

/** validate checks email syntax, real birthdays and a new contact's identity. */
export function validate(draft: ContactDraft, isNew: boolean): DraftErrors {
  const errors: DraftErrors = {};
  draft.emails.forEach(({ value }, index) => {
    if (value.trim() && !/^[^\s@]+@[^\s@]+$/.test(value.trim())) {
      errors[`emails.${index}`] = "Enter an address like name@example.com.";
    }
  });
  if (!validBirthday(draft.birthday)) errors.birthday = "This day doesn't exist.";
  if (
    isNew &&
    ![draft.givenName, draft.familyName, draft.organization].some((value) => value.trim()) &&
    !draft.emails.some(({ value }) => value.trim())
  ) {
    errors.name = "Add a name, a company or an email address.";
  }
  return errors;
}

function cleanValues(rows: LabeledValue[]): LabeledValue[] {
  return rows.map(({ label, value }) => ({ label: label.trim(), value: value.trim() })).filter((row) => row.value);
}

function cleanAddresses(rows: PostalAddress[]): PostalAddress[] {
  return rows
    .map((row) => ({
      label: row.label.trim(),
      street: row.street.trim(),
      locality: row.locality.trim(),
      region: row.region.trim(),
      postcode: row.postcode.trim(),
      country: row.country.trim(),
    }))
    .filter(({ street, locality, region, postcode, country }) => street || locality || region || postcode || country);
}

function sameRows<T extends LabeledValue | PostalAddress>(left: T[], right: T[]): boolean {
  return (
    left.length === right.length &&
    left.every((row, index) => {
      const other = right[index];
      return other !== undefined && (Object.keys(row) as (keyof T)[]).every((key) => row[key] === other[key]);
    })
  );
}

/** changedFields builds a trimmed patch, preserving the note's whitespace. */
export function changedFields(contact: Contact | null, draft: ContactDraft): ContactFields {
  const fields: ContactFields = {};
  for (const key of ["givenName", "familyName", "nickname", "organization", "title", "note"] as const) {
    const value = key === "note" ? draft[key] : draft[key].trim();
    if (contact ? value !== contact[key] : value !== "") fields[key] = value;
  }
  for (const key of ["emails", "phones", "urls"] as const) {
    const rows = cleanValues(draft[key]);
    if (contact ? !sameRows(rows, contact[key]) : rows.length > 0) fields[key] = rows;
  }
  const addresses = cleanAddresses(draft.addresses);
  if (contact ? !sameRows(addresses, contact.addresses) : addresses.length > 0) fields.addresses = addresses;
  const birthday = birthdayValue(draft.birthday);
  if (contact ? birthday !== contact.birthday : birthday !== "") fields.birthday = birthday;
  return fields;
}
