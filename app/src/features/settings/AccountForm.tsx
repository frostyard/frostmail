// The account form of the settings window's Accounts pane
// (docs/specs/settings-ui.md, AccountForm). Task T-0057 implements it; the
// stub draws nothing.
import type { AccountKind, AuthKind, DiscoverySource, ServerConfig } from "../../rpc/gen/api";

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
}

/** AccountForm edits a new or an existing account. */
export function AccountForm(_props: AccountFormProps) {
  return null;
}
