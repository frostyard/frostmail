// The Signatures pane of the settings window (docs/specs/settings-ui.md,
// IdentityEditor). Task T-0058 implements it; the stub draws nothing.
import type { Account, Identity } from "../../rpc/gen/api";

/** IdentityChange is what saving an identity sends. */
export interface IdentityChange {
  name: string;
  replyTo: string;
  signatureHtml: string;
}

/** IdentityEditorProps are the identity editor's inputs. */
export interface IdentityEditorProps {
  accounts: Account[];
  identities: Identity[];
  busy: boolean;
  error: string | null;
  onSave: (id: number, change: IdentityChange) => void;
}

/** IdentityEditor lists identities and edits the selected one. */
export function IdentityEditor(_props: IdentityEditorProps) {
  return null;
}
