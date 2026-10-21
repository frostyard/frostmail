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

const SERVICE_NAMES: Record<ServiceKind, string> = { contacts: "Contacts", calendar: "Calendars", tasks: "Tasks" };

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
      {mode === "add" ? <AddServices value={value} busy={busy} onChange={onChange} /> : props.services}
      {error && <FormAlert text={error} />}
      <ButtonRow mode={mode} busy={busy} onSubmit={props.onSubmit} onCancel={props.onCancel} />
    </form>
  );
}
