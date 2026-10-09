// The account form of the settings window's Accounts pane
// (docs/specs/settings-ui.md, AccountForm): it adds a new account (its
// address, Find Settings, the sign-in, both servers and the options) and
// edits an existing one (its name, servers, options, a new password, or
// signing in to Google again), reporting every edit as a new value.
import type { ChangeEvent, FormEvent, ReactNode } from "react";

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

/** offeredServices are the services an account kind can turn on, in
 *  order. Task T-0093 builds it. */
export function offeredServices(_kind: AccountKind): ServiceKind[] {
  return [];
}

/** defaultServices are the services a new account of a kind starts with
 *  checked. Task T-0093 builds it. */
export function defaultServices(_kind: AccountKind): ServiceKind[] {
  return [];
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
  /** Edit mode, Google sign-in: starts it in the browser. */
  onSignIn: () => void;
  /** A request is running. */
  busy: boolean;
  /** The last request's error. */
  error: string | null;
  onSubmit: () => void;
  onCancel: () => void;
  services?: ReactNode;
}

const KINDS: readonly AccountKind[] = ["imap", "gmail", "icloud"];
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

function GoogleRow(props: { mode: "add" | "edit"; signedIn: boolean; busy: boolean; onSignIn: () => void }) {
  return (
    <>
      <span className={LABEL}>Google:</span>
      {props.mode === "add" ? (
        <p className="text-[12px] leading-4 text-secondary">
          After adding the account, sign in with Google in your browser.
        </p>
      ) : (
        <div className="flex items-center">
          <span className={`text-[13px] leading-[18px] ${props.signedIn ? "text-secondary" : "text-flag-1"}`}>
            {props.signedIn ? "Signed in" : "Not signed in"}
          </span>
          <button type="button" className={`${BUTTON} ml-3`} disabled={props.busy} onClick={props.onSignIn}>
            {props.signedIn ? "Sign In Again…" : "Sign In…"}
          </button>
        </div>
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
          if (kind) onChange({ ...value, kind, auth: kind === "gmail" ? value.auth : "password" });
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
        <option value="password">Password</option>
        {value.kind === "gmail" && <option value="oauth2">Google sign-in</option>}
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

function OptionRow(props: { text: string; checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <label className="flex items-center gap-2 text-[13px] leading-[18px]">
      <input
        type="checkbox"
        checked={props.checked}
        onChange={(event: ChangeEvent<HTMLInputElement>) => props.onChange(event.target.checked)}
      />
      {props.text}
    </label>
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

/** AccountForm edits a new or an existing account. */
export function AccountForm(props: AccountFormProps) {
  const { mode, value, onChange, busy, error } = props;
  return (
    <form
      className="flex h-full flex-1 flex-col overflow-y-auto px-6 py-5"
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
          <GoogleRow mode={mode} signedIn={props.signedIn} busy={busy} onSignIn={props.onSignIn} />
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
      {mode === "edit" && props.services}
      {error && (
        <p className={ALERT} role="alert">
          {error}
        </p>
      )}
      <ButtonRow mode={mode} busy={busy} onSubmit={props.onSubmit} onCancel={props.onCancel} />
    </form>
  );
}
