import { MinusCircle } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import {
  type Birthday,
  type ContactDraft,
  changedFields,
  type DraftErrors,
  draftOf,
  LABELS,
  type ListKind,
  nextLabel,
  validate,
} from "../../lib/contactDraft";
import type { Contact, ContactFields, PostalAddress } from "../../rpc/gen/api";
import { BUTTON, FIELD, PRIMARY_BUTTON } from "../settings/labels";

/** BookChoice names an address book available for a new contact. */
export interface BookChoice {
  id: number;
  label: string;
}

/** ContactEditorProps supplies a contact and callbacks without data access. */
export interface ContactEditorProps {
  contact: Contact | null;
  books?: BookChoice[];
  photo?: ReactNode;
  busy?: boolean;
  error?: string;
  onSave: (fields: ContactFields, bookId?: number) => void;
  onCancel: () => void;
  onDelete?: () => void;
}

const ROW = "grid min-h-8 grid-cols-[96px_minmax(0,1fr)] items-start gap-x-3";
const LABEL = "pt-[6px] text-right text-[12px] leading-4 text-secondary";
const INPUT = `${FIELD} min-w-0 w-full aria-invalid:border-flag-1`;
const MESSAGE = "mt-1 text-[12px] leading-4 text-flag-1";
const CUSTOM = "\u0000custom";
const NAMES = { email: "Email", phone: "Phone", address: "Address", url: "URL" };
const TEXT_FIELDS = [
  ["givenName", "First"],
  ["familyName", "Last"],
  ["nickname", "Nickname"],
  ["organization", "Company"],
  ["title", "Job Title"],
] as const;
const ADDRESS_FIELDS = [
  ["street", "Street"],
  ["locality", "City"],
  ["region", "State"],
  ["postcode", "ZIP"],
  ["country", "Country"],
] as const;

function TextRow({
  label,
  value,
  onChange,
  error,
  multiline = false,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  error?: string;
  multiline?: boolean;
}) {
  const id = useId();
  const props = {
    id,
    value,
    "aria-invalid": Boolean(error),
    "aria-describedby": error ? `${id}-error` : undefined,
    onChange: (event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => onChange(event.target.value),
  };
  return (
    <div className={ROW}>
      <label htmlFor={id} className={LABEL}>
        {label}
      </label>
      <div>
        {multiline ? (
          <textarea {...props} className={`${INPUT} min-h-20 py-1`} />
        ) : (
          <input {...props} className={INPUT} />
        )}
        {error && (
          <p id={`${id}-error`} className={MESSAGE}>
            {error}
          </p>
        )}
      </div>
    </div>
  );
}

function ListRows<T extends { label: string }>({
  kind,
  rows,
  onChange,
  emptyRow,
  renderValue,
}: {
  kind: ListKind;
  rows: T[];
  onChange: (rows: T[]) => void;
  emptyRow: (label: string) => T;
  renderValue: (row: T, index: number, change: (row: T) => void) => ReactNode;
}) {
  const [custom, setCustom] = useState<number[]>([]);
  const labels: readonly string[] = LABELS[kind];
  const rowKind = kind === "url" ? "URL" : kind;
  function change(index: number, row: T) {
    onChange(rows.map((current, position) => (position === index ? row : current)));
  }
  function remove(index: number) {
    setCustom(
      custom.filter((position) => position !== index).map((position) => (position > index ? position - 1 : position)),
    );
    onChange(rows.filter((_, position) => position !== index));
  }
  return (
    <div className="space-y-[6px]">
      {rows.map((row, index) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: Contact rows have no IDs; their controls follow list positions.
        <div key={index} className={ROW}>
          <div className="flex justify-end">
            {custom.includes(index) ? (
              <input
                aria-label={`Custom label for ${rowKind} ${index + 1}`}
                className={`${FIELD} w-20`}
                value={row.label}
                onChange={(event) => change(index, { ...row, label: event.target.value })}
              />
            ) : (
              <select
                aria-label={`Label for ${rowKind} ${index + 1}`}
                className={`${FIELD} w-20`}
                value={row.label}
                onChange={(event) => {
                  if (event.target.value === CUSTOM) {
                    setCustom([...custom, index]);
                    change(index, { ...row, label: "" });
                  } else change(index, { ...row, label: event.target.value });
                }}
              >
                {!labels.includes(row.label) && <option value={row.label}>{row.label}</option>}
                {labels.map((label) => (
                  <option key={label} value={label}>
                    {label}
                  </option>
                ))}
                <option value={CUSTOM}>Custom…</option>
              </select>
            )}
          </div>
          <div className="flex items-start gap-[6px]">
            <div className="min-w-0 flex-1">{renderValue(row, index, (value) => change(index, value))}</div>
            <button
              type="button"
              aria-label={`Remove ${rowKind} ${index + 1}`}
              className="mt-[6px] text-secondary"
              onClick={() => remove(index)}
            >
              <MinusCircle size={16} aria-hidden="true" />
            </button>
          </div>
        </div>
      ))}
      <div className={ROW}>
        <button
          type="button"
          className="col-start-2 justify-self-start text-[12px] leading-4 text-accent hover:underline"
          onClick={() =>
            onChange([
              ...rows,
              emptyRow(
                nextLabel(
                  kind,
                  rows.map((row) => row.label),
                ),
              ),
            ])
          }
        >
          add {rowKind}
        </button>
      </div>
    </div>
  );
}

function AddressFields({
  row,
  index,
  onChange,
}: {
  row: PostalAddress;
  index: number;
  onChange: (row: PostalAddress) => void;
}) {
  return (
    <div className="space-y-1">
      {ADDRESS_FIELDS.map(([key, label]) => {
        const props = {
          "aria-label": `${label} ${index + 1}`,
          placeholder: label,
          value: row[key],
          onChange: (event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
            onChange({ ...row, [key]: event.target.value }),
        };
        return key === "street" ? (
          <textarea key={key} {...props} rows={2} className={`${INPUT} h-auto py-1`} />
        ) : (
          <input key={key} {...props} className={INPUT} />
        );
      })}
    </div>
  );
}

function BirthdayRow({
  value,
  onChange,
  error,
}: {
  value: Birthday;
  onChange: (value: Birthday) => void;
  error?: string;
}) {
  const id = useId();
  const invalid = { "aria-invalid": Boolean(error), "aria-describedby": error ? `${id}-error` : undefined };
  return (
    <div className={ROW}>
      <span className={LABEL}>Birthday</span>
      <div>
        <div className="flex gap-[6px]">
          {(["month", "day"] as const).map((part) => (
            <select
              key={part}
              aria-label={`Birthday ${part}`}
              {...invalid}
              className={`${FIELD} aria-invalid:border-flag-1`}
              value={value[part]}
              onChange={(event) => onChange({ ...value, [part]: event.target.value })}
            >
              <option value="" />
              {Array.from({ length: part === "month" ? 12 : 31 }, (_, index) => String(index + 1).padStart(2, "0")).map(
                (number) => (
                  <option key={number} value={number}>
                    {number}
                  </option>
                ),
              )}
            </select>
          ))}
          <input
            aria-label="Birthday year"
            {...invalid}
            className={`${FIELD} w-16 aria-invalid:border-flag-1`}
            inputMode="numeric"
            value={value.year}
            onChange={(event) => onChange({ ...value, year: event.target.value })}
          />
        </div>
        {error && (
          <p id={`${id}-error`} className={MESSAGE}>
            {error}
          </p>
        )}
      </div>
    </div>
  );
}

/** ContactEditor edits a local draft and submits only the fields that changed. */
export function ContactEditor({
  contact,
  books = [],
  photo,
  busy,
  error,
  onSave,
  onCancel,
  onDelete,
}: ContactEditorProps) {
  const [draft, setDraft] = useState(() => draftOf(contact));
  const [errors, setErrors] = useState<DraftErrors>({});
  const [validated, setValidated] = useState(false);
  const [bookId, setBookId] = useState(books[0]?.id);
  const id = useId();
  function change<K extends keyof ContactDraft>(key: K, value: ContactDraft[K]) {
    const next = { ...draft, [key]: value };
    setDraft(next);
    if (validated) setErrors(validate(next, contact === null));
  }
  return (
    <form
      aria-label="Contact editor"
      noValidate
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          event.preventDefault();
          event.stopPropagation();
          onCancel();
        }
      }}
      onSubmit={(event) => {
        event.preventDefault();
        if (busy) return;
        const nextErrors = validate(draft, contact === null);
        setErrors(nextErrors);
        setValidated(true);
        if (Object.keys(nextErrors).length === 0)
          onSave(changedFields(contact, draft), contact === null ? bookId : undefined);
      }}
    >
      {photo}
      <div className="space-y-[6px]">
        {contact === null && books.length > 1 && (
          <div className={ROW}>
            <label className={LABEL} htmlFor={`${id}-book`}>
              Address Book
            </label>
            <select
              id={`${id}-book`}
              className={INPUT}
              value={bookId}
              onChange={(event) => setBookId(Number(event.target.value))}
            >
              {books.map((book) => (
                <option key={book.id} value={book.id}>
                  {book.label}
                </option>
              ))}
            </select>
          </div>
        )}
        {TEXT_FIELDS.map(([key, label]) => (
          <TextRow
            key={key}
            label={label}
            value={draft[key]}
            onChange={(value) => change(key, value)}
            error={key === "givenName" ? errors.name : undefined}
          />
        ))}
        {(["email", "phone", "address", "url"] as const).map((kind) => {
          if (kind === "address")
            return (
              <ListRows
                key={kind}
                kind={kind}
                rows={draft.addresses}
                onChange={(rows) => change("addresses", rows)}
                emptyRow={(label) => ({ label, street: "", locality: "", region: "", postcode: "", country: "" })}
                renderValue={(row, index, update) => <AddressFields row={row} index={index} onChange={update} />}
              />
            );
          const key = kind === "email" ? "emails" : kind === "phone" ? "phones" : "urls";
          return (
            <ListRows
              key={kind}
              kind={kind}
              rows={draft[key]}
              onChange={(rows) => change(key, rows)}
              emptyRow={(label) => ({ label, value: "" })}
              renderValue={(row, index, update) => {
                const message = errors[`${key}.${index}`];
                const errorId = `${id}-${key}-${index}-error`;
                return (
                  <>
                    <input
                      aria-label={`${NAMES[kind]} ${index + 1}`}
                      className={INPUT}
                      value={row.value}
                      aria-invalid={Boolean(message)}
                      aria-describedby={message ? errorId : undefined}
                      onChange={(event) => update({ ...row, value: event.target.value })}
                    />
                    {message && (
                      <p id={errorId} className={MESSAGE}>
                        {message}
                      </p>
                    )}
                  </>
                );
              }}
            />
          );
        })}
        <BirthdayRow value={draft.birthday} onChange={(value) => change("birthday", value)} error={errors.birthday} />
        <TextRow label="Note" value={draft.note} onChange={(value) => change("note", value)} multiline />
      </div>
      <footer className="mt-4 flex items-center gap-2">
        {onDelete && (
          <button type="button" className="text-[13px] leading-4 text-flag-1 hover:underline" onClick={onDelete}>
            Delete Contact
          </button>
        )}
        <button type="button" className={`${BUTTON} ml-auto`} onClick={onCancel}>
          Cancel
        </button>
        <button type="submit" className={PRIMARY_BUTTON} disabled={busy || Object.keys(errors).length > 0}>
          Done
        </button>
      </footer>
      {error && (
        <div role="alert" className={MESSAGE}>
          {error}
        </div>
      )}
    </form>
  );
}
