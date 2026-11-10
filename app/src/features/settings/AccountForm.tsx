// The account form of the settings window's Accounts pane
// (docs/specs/settings-ui.md, AccountForm): it adds a new account (its
// address, Find Settings, the sign-in, both servers and the options) and
// edits an existing one (its name, servers, options, a new password, or
// signing in to Google or Microsoft again), reporting every edit as a new
// value.
import { type ChangeEvent, type FormEvent, type ReactNode, useEffect, useId, useRef, useState } from "react";

import { pasteSignIn } from "../../app/settings";
import type { AccountKind, AuthKind, DiscoverySource, ServerConfig, ServiceKind } from "../../rpc/gen/api";
import { ALERT, BUTTON, FIELD, GRID, KIND_LABEL, LABEL, PRIMARY_BUTTON } from "./labels";
import { ServerFields } from "./ServerFields";

/** AccountFormValue is what the form edits. */
export interface AccountFormValue {
  kind: AccountKind;
  email: string;
  displayName: string;
  auth: AuthKind;
  imap: ServerConfig;
  smtp: ServerConfig;
  readOnly: boolean;
  notify: boolean;
  /** The sync window in days (ADR-0016); 0 keeps every message, and an
   * absent value means 0, as a new account starts. */
  syncDays?: number;
  /** How long mail stays in the account's trash mailbox before maild erases
   * it; 0 or absent is Never (ADR-0051, plan 0014 Phase 4). */
  eraseTrashDays?: 0 | 1 | 7 | 30;
  /** The same for the account's junk mailbox (ADR-0051). */
  eraseJunkDays?: 0 | 1 | 7 | 30;
  /** A new password; empty keeps the stored one. */
  password: string;
  /** Add mode: the services turned on with mail; absent means none. */
  services?: ServiceKind[];
}

/** offeredServices are the services an account kind can turn on, in order. */
export function offeredServices(kind: AccountKind): ServiceKind[] {
  if (kind === "icloud" || kind === "microsoft") return ["contacts", "calendar"];
  return ["contacts", "calendar", "tasks"];
}

/** defaultServices are the services a new account of a kind starts with checked. */
export function defaultServices(kind: AccountKind): ServiceKind[] {
  return kind === "gmail" || kind === "icloud" || kind === "microsoft" ? offeredServices(kind) : [];
}

/** DiscoveryState is how far finding a new account's servers got. */
export type DiscoveryState =
  | { kind: "idle" }
  | { kind: "finding" }
  | { kind: "found"; source: DiscoverySource }
  | { kind: "failed"; message: string };

/** AccountFormProps are the account form's inputs. */
export interface AccountFormProps {
  mode: "add" | "edit";
  value: AccountFormValue;
  onChange: (v: AccountFormValue) => void;
  /** Add mode: the state of Find Settings. */
  discovery: DiscoveryState;
  onDiscover: () => void;
  /** Edit mode: whether the stored credential works. */
  signedIn: boolean;
  /** Edit mode, Google or Microsoft sign-in: starts it in the browser. */
  onSignIn: () => void;
  /** Edit mode: a sign-in this window started waits for the shown account. */
  signInWaiting?: boolean;
  /** Finishes a waiting sign-in with the address of the page the browser could not open. */
  onFinishSignIn?: (url: string) => Promise<void>;
  /** A request is running. */
  busy: boolean;
  /** The last request's error. */
  error: string | null;
  onSubmit: () => void;
  onCancel: () => void;
  services?: ReactNode;
  /** Edit mode: the names of the account's trash and junk mailboxes, which
   * its Mailbox Behaviors erase (ADR-0051). */
  eraseMailboxes?: { trash?: string; junk?: string };
}

const KINDS: readonly AccountKind[] = ["imap", "gmail", "microsoft", "icloud"];

/** authFor is the sign-in a new kind starts with: Gmail keeps the one
 * chosen, Microsoft signs in only with OAuth, the rest with a password. */
function authFor(kind: AccountKind, current: AuthKind): AuthKind {
  if (kind === "gmail") return current;
  return kind === "microsoft" ? "oauth2" : "password";
}

/** PROVIDER names the OAuth provider of the kinds that sign in with one. */
const PROVIDER: Partial<Record<AccountKind, string>> = { gmail: "Google", microsoft: "Microsoft" };
const EMAIL_RE = /^[^@\s]+@[^@\s]+$/;

function headingFor(mode: "add" | "edit", value: AccountFormValue): string {
  if (mode === "add") return "Add Account";
  return value.displayName.trim() === "" ? value.email : value.displayName;
}

function discoveryStatus(discovery: DiscoveryState): { text: string; tone: string } | null {
  switch (discovery.kind) {
    case "finding":
      return { text: "Looking up servers…", tone: "text-secondary" };
    case "found":
      return discovery.source === "none"
        ? { text: "No settings found; enter the servers below.", tone: "text-secondary" }
        : { text: "Found settings for this address.", tone: "text-secondary" };
    case "failed":
      return { text: discovery.message, tone: "text-flag-1" };
    default:
      return null;
  }
}

function FindSettingsRow(props: { email: string; discovery: DiscoveryState; busy: boolean; onDiscover: () => void }) {
  const status = discoveryStatus(props.discovery);
  const disabled = props.busy || props.discovery.kind === "finding" || !EMAIL_RE.test(props.email.trim());
  return (
    <div className="col-start-2 flex items-center gap-2">
      <button type="button" className={BUTTON} disabled={disabled} onClick={props.onDiscover}>
        Find Settings
      </button>
      {status && <span className={`text-[12px] leading-4 ${status.tone}`}>{status.text}</span>}
    </div>
  );
}

function passwordHint(kind: AccountKind): string | null {
  if (kind === "gmail") return "Use an app password from your Google account.";
  if (kind === "icloud") return "Use an app-specific password from appleid.apple.com.";
  return null;
}

function PasswordRow(props: { value: AccountFormValue; editing: boolean; onChange: (v: AccountFormValue) => void }) {
  const hint = passwordHint(props.value.kind);
  return (
    <>
      <label className={LABEL} htmlFor="account-password">
        Password:
      </label>
      <input
        id="account-password"
        type="password"
        className={FIELD}
        autoComplete="new-password"
        placeholder={props.editing ? "Unchanged" : undefined}
        value={props.value.password}
        onChange={(event: ChangeEvent<HTMLInputElement>) =>
          props.onChange({ ...props.value, password: event.target.value })
        }
      />
      {hint && <p className="col-start-2 text-[12px] leading-4 text-secondary">{hint}</p>}
    </>
  );
}

function FinishSignInRow(props: { busy: boolean; onFinish: (url: string) => Promise<void> }) {
  const id = useId();
  const [address, setAddress] = useState("");
  const [sending, setSending] = useState(false);
  const busy = props.busy || sending;
  const finish = async () => {
    const url = address.trim();
    if (url === "" || busy) return;
    setSending(true);
    // The row goes once the account is signed in; after an error the
    // address stays, to correct or send again.
    try {
      await props.onFinish(url);
    } finally {
      setSending(false);
    }
  };
  return (
    <div className="col-start-2 flex flex-col gap-2">
      <p className="text-[12px] leading-4 text-secondary">
        If your browser cannot open the page after you sign in, copy that page's address and paste it here.
      </p>
      <div className="flex items-center gap-2">
        <label htmlFor={id} className={LABEL}>
          Address
        </label>
        <input
          id={id}
          type="url"
          className={`${FIELD} min-w-0 flex-1`}
          autoComplete="off"
          spellCheck={false}
          disabled={busy}
          value={address}
          onChange={(event: ChangeEvent<HTMLInputElement>) => setAddress(event.target.value)}
        />
        <button type="button" className={BUTTON} disabled={busy || address.trim() === ""} onClick={() => void finish()}>
          Finish Sign-In
        </button>
      </div>
    </div>
  );
}

function SignInRow(props: {
  kind: AccountKind;
  mode: "add" | "edit";
  signedIn: boolean;
  busy: boolean;
  onSignIn: () => void;
  waiting?: boolean;
  onFinishSignIn?: (url: string) => Promise<void>;
}) {
  const provider = PROVIDER[props.kind] ?? "Google";
  const covers = offeredServices(props.kind).length > 0 ? ", once for mail and the services checked below" : "";
  return (
    <>
      <span className={LABEL}>{provider}:</span>
      {props.mode === "add" ? (
        <p className="text-[12px] leading-4 text-secondary">
          After adding the account, sign in with {provider} in your browser{covers}.
        </p>
      ) : (
        <>
          <div className="flex items-center">
            <span className={`text-[13px] leading-[18px] ${props.signedIn ? "text-secondary" : "text-flag-1"}`}>
              {props.signedIn ? "Signed in" : "Not signed in"}
            </span>
            <button type="button" className={`${BUTTON} ml-3`} disabled={props.busy} onClick={props.onSignIn}>
              {props.signedIn ? "Sign In Again…" : "Sign In…"}
            </button>
          </div>
          {props.waiting && props.onFinishSignIn && pasteSignIn() && (
            <FinishSignInRow busy={props.busy} onFinish={props.onFinishSignIn} />
          )}
        </>
      )}
    </>
  );
}

function IdentityRows(props: AccountFormProps) {
  const { mode, value, onChange } = props;
  return (
    <>
      <label className={LABEL} htmlFor="account-email">
        Email Address:
      </label>
      <input
        id="account-email"
        type="email"
        className={FIELD}
        autoComplete="off"
        spellCheck={false}
        readOnly={mode === "edit"}
        value={value.email}
        onChange={(event: ChangeEvent<HTMLInputElement>) => onChange({ ...value, email: event.target.value })}
      />
      {mode === "add" && (
        <FindSettingsRow
          email={value.email}
          discovery={props.discovery}
          busy={props.busy}
          onDiscover={props.onDiscover}
        />
      )}
      <label className={LABEL} htmlFor="account-name">
        Full Name:
      </label>
      <input
        id="account-name"
        type="text"
        className={FIELD}
        autoComplete="off"
        spellCheck={false}
        value={value.displayName}
        onChange={(event: ChangeEvent<HTMLInputElement>) => onChange({ ...value, displayName: event.target.value })}
      />
    </>
  );
}

function KindRows(props: AccountFormProps) {
  const { value, onChange } = props;
  const editing = props.mode === "edit";
  return (
    <>
      <label className={LABEL} htmlFor="account-kind">
        Account Type:
      </label>
      <select
        id="account-kind"
        className={FIELD}
        value={value.kind}
        disabled={editing}
        onChange={(event: ChangeEvent<HTMLSelectElement>) => {
          const kind = KINDS.find((candidate) => candidate === event.target.value);
          if (kind)
            onChange({
              ...value,
              kind,
              auth: authFor(kind, value.auth),
              ...(editing ? {} : { services: defaultServices(kind) }),
            });
        }}
      >
        {KINDS.map((kind) => (
          <option key={kind} value={kind}>
            {KIND_LABEL[kind]}
          </option>
        ))}
      </select>
      <label className={LABEL} htmlFor="account-auth">
        Sign In:
      </label>
      <select
        id="account-auth"
        className={FIELD}
        value={value.auth}
        disabled={editing}
        onChange={(event: ChangeEvent<HTMLSelectElement>) => {
          const auth: AuthKind = event.target.value === "oauth2" ? "oauth2" : "password";
          onChange({ ...value, auth });
        }}
      >
        {value.kind !== "microsoft" && <option value="password">Password</option>}
        {value.kind === "gmail" && <option value="oauth2">Google sign-in</option>}
        {value.kind === "microsoft" && <option value="oauth2">Microsoft sign-in</option>}
      </select>
    </>
  );
}

function ServerRows(props: { value: AccountFormValue; onChange: (v: AccountFormValue) => void; busy: boolean }) {
  return (
    <>
      <div className="mt-5">
        <ServerFields
          id="imap"
          legend="Incoming Mail (IMAP)"
          value={props.value.imap}
          onChange={(imap: ServerConfig) => props.onChange({ ...props.value, imap })}
          disabled={props.busy}
        />
      </div>
      <div className="mt-5">
        <ServerFields
          id="smtp"
          legend="Outgoing Mail (SMTP)"
          value={props.value.smtp}
          onChange={(smtp: ServerConfig) => props.onChange({ ...props.value, smtp })}
          disabled={props.busy}
        />
      </div>
    </>
  );
}

function OptionRow(props: {
  text: string;
  checked: boolean;
  disabled?: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label className="flex items-center gap-2 text-[13px] leading-[18px]">
      <input
        type="checkbox"
        checked={props.checked}
        disabled={props.disabled}
        onChange={(event: ChangeEvent<HTMLInputElement>) => props.onChange(event.target.checked)}
      />
      {props.text}
    </label>
  );
}

/** SYNC_WINDOWS are the "Keep mail from" choices, in the order offered (ADR-0016). */
const SYNC_WINDOWS: readonly { days: number; label: string }[] = [
  { days: 0, label: "All mail" },
  { days: 30, label: "Last 30 days" },
  { days: 90, label: "Last 90 days" },
  { days: 183, label: "Last 6 months" },
  { days: 365, label: "Last year" },
  { days: 730, label: "Last 2 years" },
];

/**
 * SyncWindowRows set the account's sync window (docs/specs/settings-ui.md,
 * AccountForm; ADR-0016). A value set outside the list, with `mailctl
 * account set --sync-days`, keeps an option of its own, so it stays shown.
 */
function SyncWindowRows(props: { value: AccountFormValue; busy: boolean; onChange: (v: AccountFormValue) => void }) {
  const days = props.value.syncDays ?? 0;
  const windows = SYNC_WINDOWS.some((window) => window.days === days)
    ? SYNC_WINDOWS
    : [...SYNC_WINDOWS, { days, label: `Last ${days} days` }];
  return (
    <div className={`mt-5 ${GRID}`}>
      <label className={LABEL} htmlFor="account-sync-days">
        Keep mail from:
      </label>
      <select
        id="account-sync-days"
        className={FIELD}
        disabled={props.busy}
        value={String(days)}
        onChange={(event: ChangeEvent<HTMLSelectElement>) =>
          props.onChange({ ...props.value, syncDays: Number(event.target.value) })
        }
      >
        {windows.map((window) => (
          <option key={window.days} value={String(window.days)}>
            {window.label}
          </option>
        ))}
      </select>
      <p className="col-start-2 text-[12px] leading-4 text-secondary">
        A longer time downloads older mail from the server. A shorter time removes older mail from this computer only;
        it stays on the server.
      </p>
    </div>
  );
}

const SERVICE_NAMES: Record<ServiceKind, string> = { contacts: "Contacts", calendar: "Calendars", tasks: "Tasks" };

/** ERASE_DAYS are the choices of each mailbox behavior, in the order offered (ADR-0051). */
const ERASE_DAYS: readonly { days: 0 | 1 | 7 | 30; label: string }[] = [
  { days: 0, label: "Never" },
  { days: 1, label: "After One Day" },
  { days: 7, label: "After One Week" },
  { days: 30, label: "After One Month" },
];

/** eraseNote says what the behaviors erase, naming the mailboxes the account has. */
function eraseNote(names: string[]): string {
  const quoted = names.map((name) => `“${name}”`).join(" or ");
  return (
    `Messages are erased from the server after they have been in ${quoted} that long, and cannot be recovered. ` +
    "Mail older than this account's sync window is left on the server."
  );
}

/**
 * MailboxBehaviorRows set how long mail stays in the account's trash and junk
 * mailboxes (docs/specs/settings-ui.md, AccountForm; ADR-0051). A behavior
 * whose mailbox the account does not have is not offered, and a read-only
 * account never erases, so both are disabled then.
 */
function MailboxBehaviorRows(props: {
  value: AccountFormValue;
  busy: boolean;
  eraseMailboxes?: { trash?: string; junk?: string };
  onChange: (v: AccountFormValue) => void;
}) {
  const heading = useId();
  const names = [props.eraseMailboxes?.trash, props.eraseMailboxes?.junk].filter(
    (name): name is string => name !== undefined && name !== "",
  );
  // A behavior the account has no mailbox for is not offered: it says
  // Never, whatever the stored value is.
  const row = (
    id: string,
    label: string,
    mailbox: string | undefined,
    stored: 0 | 1 | 7 | 30,
    onChoose: (days: 0 | 1 | 7 | 30) => void,
  ) => (
    <>
      <label className={LABEL} htmlFor={id}>
        {label}
      </label>
      <select
        id={id}
        className={FIELD}
        disabled={props.busy || props.value.readOnly || mailbox === undefined}
        value={String(mailbox === undefined ? 0 : stored)}
        onChange={(event: ChangeEvent<HTMLSelectElement>) => {
          const chosen = ERASE_DAYS.find((option) => String(option.days) === event.target.value);
          if (chosen) onChoose(chosen.days);
        }}
      >
        {ERASE_DAYS.map((option) => (
          <option key={option.days} value={String(option.days)}>
            {option.label}
          </option>
        ))}
      </select>
    </>
  );
  return (
    <section className="mt-5" aria-labelledby={heading}>
      <h3 id={heading} className="mb-2 text-[13px] leading-[18px] font-semibold">
        Mailbox Behaviors
      </h3>
      <div className={GRID}>
        {row(
          "account-erase-trash",
          "Erase deleted messages:",
          props.eraseMailboxes?.trash,
          props.value.eraseTrashDays ?? 0,
          (days) => props.onChange({ ...props.value, eraseTrashDays: days }),
        )}
        {row(
          "account-erase-junk",
          "Erase junk messages:",
          props.eraseMailboxes?.junk,
          props.value.eraseJunkDays ?? 0,
          (days) => props.onChange({ ...props.value, eraseJunkDays: days }),
        )}
        {names.length > 0 && <p className="col-start-2 text-[12px] leading-4 text-secondary">{eraseNote(names)}</p>}
      </div>
    </section>
  );
}

function AddServices(props: { value: AccountFormValue; busy: boolean; onChange: (v: AccountFormValue) => void }) {
  const heading = useId();
  const offered = offeredServices(props.value.kind);
  const selected = props.value.services ?? [];
  if (offered.length === 0) return null;
  return (
    <section aria-labelledby={heading}>
      <h3 id={heading} className="mt-5 mb-2 text-[13px] leading-[18px] font-semibold">
        Contacts, Calendars and Tasks
      </h3>
      <div className="flex flex-col gap-2">
        {offered.map((service) => (
          <OptionRow
            key={service}
            text={SERVICE_NAMES[service]}
            checked={selected.includes(service)}
            disabled={props.busy}
            onChange={(checked) =>
              props.onChange({
                ...props.value,
                services: offered.filter((kind) => (kind === service ? checked : selected.includes(kind))),
              })
            }
          />
        ))}
        <p className="text-[12px] leading-4 text-secondary">
          {props.value.auth === "oauth2"
            ? `${PROVIDER[props.value.kind] ?? "Google"} asks once for mail and the services checked here.`
            : "Frostmail finds them with the account's password."}
        </p>
      </div>
    </section>
  );
}

function ButtonRow(props: { mode: "add" | "edit"; busy: boolean; onSubmit: () => void; onCancel: () => void }) {
  return (
    <div className="mt-auto flex justify-end gap-2 pt-5">
      {props.mode === "add" && (
        <button type="button" className={BUTTON} onClick={props.onCancel}>
          Cancel
        </button>
      )}
      <button type="submit" className={PRIMARY_BUTTON} disabled={props.busy}>
        {props.mode === "edit" ? "Save" : "Add Account"}
      </button>
    </div>
  );
}

/**
 * FormAlert shows the last request's error and scrolls it into view: it
 * sits below the services, out of sight when the window is shorter than
 * the form, where a failed Sign In… seemed to do nothing.
 */
function FormAlert({ text }: { text: string }) {
  const ref = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    if (text) ref.current?.scrollIntoView?.({ block: "nearest" });
  }, [text]);
  return (
    <p ref={ref} className={ALERT} role="alert">
      {text}
    </p>
  );
}

/** AccountForm edits a new or an existing account. */
export function AccountForm(props: AccountFormProps) {
  const { mode, value, onChange, busy, error } = props;
  return (
    <form
      className="flex h-full flex-1 flex-col overflow-y-auto px-6 py-5 narrow:min-h-0 narrow:px-4"
      onSubmit={(event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        props.onSubmit();
      }}
    >
      <h2 className="mb-4 text-[15px] leading-5 font-semibold">{headingFor(mode, value)}</h2>
      <div className={GRID}>
        <IdentityRows {...props} />
        <KindRows {...props} />
        {value.auth === "password" ? (
          <PasswordRow value={value} editing={mode === "edit"} onChange={onChange} />
        ) : (
          <SignInRow
            kind={value.kind}
            mode={mode}
            signedIn={props.signedIn}
            busy={busy}
            onSignIn={props.onSignIn}
            waiting={props.signInWaiting}
            onFinishSignIn={props.onFinishSignIn}
          />
        )}
      </div>
      <ServerRows value={value} onChange={onChange} busy={busy} />
      <div className="mt-5 flex flex-col gap-2">
        <OptionRow
          text="Read only: never change anything on the server"
          checked={value.readOnly}
          onChange={(readOnly) => onChange({ ...value, readOnly })}
        />
        <OptionRow
          text="Notify me about new mail"
          checked={value.notify}
          onChange={(notify) => onChange({ ...value, notify })}
        />
      </div>
      {mode === "edit" && <SyncWindowRows value={value} busy={busy} onChange={onChange} />}
      {mode === "edit" && (
        <MailboxBehaviorRows value={value} busy={busy} eraseMailboxes={props.eraseMailboxes} onChange={onChange} />
      )}
      {mode === "add" ? <AddServices value={value} busy={busy} onChange={onChange} /> : props.services}
      {error && <FormAlert text={error} />}
      <ButtonRow mode={mode} busy={busy} onSubmit={props.onSubmit} onCancel={props.onCancel} />
    </form>
  );
}
