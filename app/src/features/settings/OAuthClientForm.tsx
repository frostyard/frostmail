// The Sign-In pane of the settings window (docs/specs/settings-ui.md,
// OAuthClientForm). Task T-0059 implements it; the stub draws nothing.
import type { OAuthClient } from "../../rpc/gen/api";

/** OAuthClientFormProps are the Google client form's inputs. */
export interface OAuthClientFormProps {
  /** The stored Google client, or null when there is none. */
  client: OAuthClient | null;
  busy: boolean;
  error: string | null;
  /** Saves the client; an empty secret keeps the stored one. */
  onSave: (clientId: string, clientSecret: string) => void;
}

/** OAuthClientForm edits the Google client. */
export function OAuthClientForm(_props: OAuthClientFormProps) {
  return null;
}
