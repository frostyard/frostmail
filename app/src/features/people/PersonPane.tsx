// The People module's detail pane (docs/specs/pim-ui.md, Person pane).
// Contact fields stay in source order, with actions delegated to the caller.
import { Mail } from "lucide-react";
import { type ReactNode, useId } from "react";

import { formatListDate } from "../../lib/format";
import type { Contact, MessageSummary, Person, PostalAddress } from "../../rpc/gen/api";

import { Avatar } from "./Avatar";

/** BookLabel names a contact's address book and account. */
export interface BookLabel {
  name: string;
  account: string;
}

/** PersonPaneProps are the person pane's inputs. */
export interface PersonPaneProps {
  /** null shows "No Contact Selected". */
  person: Person | null;
  /** Address book and account names by collection ID. */
  books: Record<number, BookLabel>;
  /** The person's photo as a data: URL, when loaded. */
  photo?: string;
  /** Recent mail with the person's first email. */
  recent: MessageSummary[];
  upcoming?: ReactNode;
  /** The clock for the recent mail's dates. */
  now: Date;
  onCompose: (email: string) => void;
  onOpenMessage: (id: number) => void;
  onOpenURL: (url: string) => void;
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt className="text-right text-[12px] leading-[18px] text-secondary">{label}</dt>
      <dd className="min-w-0 break-words text-[13px] leading-[18px] text-primary">{children}</dd>
    </>
  );
}

function addressLines(address: PostalAddress): string[] {
  return [
    ...address.street.split("\n"),
    [address.locality, address.region, address.postcode].filter(Boolean).join(" "),
    address.country,
  ].filter(Boolean);
}

// Birthdays are local calendar dates, including February 29 without a year.
function birthdayLabel(value: string): string {
  const noYear = value.startsWith("--");
  const parts = (noYear ? `2000-${value.slice(2)}` : value).split("-").map(Number);
  const [year = 2000, month = 1, day = 1] = parts;
  const date = new Date(year, month - 1, day);
  date.setFullYear(year);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, noYear ? { month: "long", day: "numeric" } : { dateStyle: "long" }).format(
    date,
  );
}

function ContactBlock({
  contact,
  book,
  onCompose,
  onOpenURL,
}: {
  contact: Contact;
  book: BookLabel | undefined;
  onCompose: (email: string) => void;
  onOpenURL: (url: string) => void;
}) {
  const headingId = useId();
  const title = `${book ? `${book.name} — ${book.account}` : contact.collectionId}${contact.readOnly ? " · Read-only" : ""}`;
  const fields: ReactNode[] = [];
  const add = (label: string, value: ReactNode) => {
    fields.push(
      <Field key={fields.length} label={label}>
        {value}
      </Field>,
    );
  };
  for (const email of contact.emails) {
    if (email.value)
      add(
        email.label || "email",
        <button type="button" className="text-accent hover:underline" onClick={() => onCompose(email.value)}>
          {email.value}
        </button>,
      );
  }
  for (const phone of contact.phones) {
    if (phone.value) add(phone.label || "phone", phone.value);
  }
  for (const address of contact.addresses) {
    const lines = addressLines(address);
    if (lines.length > 0)
      add(
        address.label || "address",
        lines.map((line) => <div key={line}>{line}</div>),
      );
  }
  for (const url of contact.urls) {
    if (!url.value) continue;
    add(
      url.label || "homepage",
      /^https?:\/\//i.test(url.value) ? (
        <a
          href={url.value}
          className="text-accent hover:underline"
          onClick={(event) => {
            event.preventDefault();
            onOpenURL(url.value);
          }}
        >
          {url.value}
        </a>
      ) : (
        url.value
      ),
    );
  }
  if (contact.birthday) add("birthday", birthdayLabel(contact.birthday));
  if (contact.nickname) add("nickname", contact.nickname);
  if (contact.note) add("note", <span className="whitespace-pre-wrap">{contact.note}</span>);
  return (
    <section aria-labelledby={headingId}>
      <h3 id={headingId} className="mb-2 text-sidebar-section text-secondary uppercase">
        {title}
      </h3>
      <dl className="grid select-text grid-cols-[96px_minmax(0,1fr)] gap-x-3 gap-y-[6px]">{fields}</dl>
    </section>
  );
}

function RecentMail({ recent, now, onOpenMessage }: Pick<PersonPaneProps, "recent" | "now" | "onOpenMessage">) {
  const headingId = useId();
  if (recent.length === 0) return null;
  return (
    <section aria-labelledby={headingId}>
      <h3 id={headingId} className="mb-2 text-sidebar-section text-secondary uppercase">
        Recent Mail
      </h3>
      {recent.map((message) => {
        const subject = message.subject.trim();
        return (
          <button
            key={message.id}
            type="button"
            className="flex h-8 w-full items-center gap-3 text-left hover:bg-selection-inactive"
            onClick={() => onOpenMessage(message.id)}
          >
            <span className={`min-w-0 flex-1 truncate text-list-subject ${subject ? "" : "text-tertiary"}`}>
              {subject || "(No Subject)"}
            </span>
            <span className="shrink-0 text-list-date text-secondary tabular-nums">
              {formatListDate(new Date(message.date), now)}
            </span>
          </button>
        );
      })}
    </section>
  );
}

/** PersonPane shows a person's contacts and recent mail. */
export function PersonPane({
  person,
  books,
  photo,
  recent,
  upcoming,
  now,
  onCompose,
  onOpenMessage,
  onOpenURL,
}: PersonPaneProps) {
  if (person === null) {
    return (
      <div className="flex h-full items-center justify-center bg-window p-6">
        <span className="text-empty text-secondary">No Contact Selected</span>
      </div>
    );
  }
  const email = person.contacts[0]?.emails[0]?.value ?? "";
  const subtitle = [person.contacts[0]?.title, person.organization].filter(Boolean).join(" · ");
  return (
    <div className="h-full overflow-y-auto bg-window p-6">
      <header className="mb-5 flex items-start gap-4">
        <Avatar name={person.displayName} email={email} photo={photo} size={64} />
        <div className="min-w-0 flex-1">
          <h2 className="select-text text-[20px] font-semibold leading-[26px]">{person.displayName}</h2>
          {subtitle && <div className="select-text text-[13px] leading-[18px] text-secondary">{subtitle}</div>}
          <button
            type="button"
            disabled={!email}
            onClick={() => onCompose(email)}
            className="mt-2 flex h-7 items-center gap-2 rounded-md bg-accent px-3 text-accent-contrast disabled:opacity-40"
          >
            <Mail size={16} aria-hidden="true" />
            Message
          </button>
        </div>
      </header>
      <div className="space-y-5">
        {person.contacts.map((contact) => (
          <ContactBlock
            key={contact.id}
            contact={contact}
            book={books[contact.collectionId]}
            onCompose={onCompose}
            onOpenURL={onOpenURL}
          />
        ))}
        <RecentMail recent={recent} now={now} onOpenMessage={onOpenMessage} />
        {upcoming}
      </div>
    </div>
  );
}
