// The Sign-In pane of the settings window (docs/specs/settings-ui.md,
// OAuthClientForm): shows whether a Google OAuth client is stored and lets
// the user enter its ID and secret.
import { useEffect, useState } from "react";

import type { OAuthClient } from "../../rpc/gen/api";
import { ALERT, FIELD, GRID, LABEL, PRIMARY_BUTTON } from "./labels";

/** OAuthClientFormProps are the Google client form's inputs. */
export interface OAuthClientFormProps {
  /** The stored Google client, or null when there is none. */
  client: OAuthClient | null;
  busy: boolean;
  error: string | null;
  /** Saves the client; an empty secret keeps the stored one. */
  onSave: (clientId: string, clientSecret: string) => void;
}

function statusText(client: OAuthClient | null): string {
  if (!client) return "No client stored";
  return client.hasSecret ? "Client and secret stored" : "Client stored without a secret";
}

/** OAuthClientForm edits the Google client. */
export function OAuthClientForm({ client, busy, error, onSave }: OAuthClientFormProps) {
  const storedId = client?.clientId ?? "";
  const [clientId, setClientId] = useState(storedId);
  const [secret, setSecret] = useState("");

  useEffect(() => setClientId(storedId), [storedId]);

  const trimmed = clientId.trim();
  const disabled = busy || trimmed === "" || (trimmed === storedId && secret === "");

  function submit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    onSave(trimmed, secret);
    setSecret("");
  }

  return (
    <form aria-label="Google client" className="flex max-w-[560px] flex-col px-6 py-5" onSubmit={submit}>
      <h2 className="mb-4 text-[15px] leading-5 font-semibold">Google Client</h2>
      <p className="mb-4 text-[12px] leading-4 text-secondary">
        Gmail sign-in uses your own Google OAuth client until Frostmail has a verified one. Create a Desktop app client
        in Google Cloud and enter it here.
      </p>
      <div className={GRID}>
        <label className={LABEL} htmlFor="oauth-client-id">
          Client ID:
        </label>
        <input
          id="oauth-client-id"
          className={FIELD}
          type="text"
          autoComplete="off"
          spellCheck={false}
          value={clientId}
          onChange={(e) => setClientId(e.target.value)}
        />
        <label className={LABEL} htmlFor="oauth-client-secret">
          Client Secret:
        </label>
        <input
          id="oauth-client-secret"
          className={FIELD}
          type="password"
          autoComplete="off"
          placeholder={client?.hasSecret ? "Stored" : undefined}
          value={secret}
          onChange={(e) => setSecret(e.target.value)}
        />
      </div>
      <p className="mt-3 text-[12px] leading-4 text-secondary">{statusText(client)}</p>
      {error ? (
        <p role="alert" className={ALERT}>
          {error}
        </p>
      ) : null}
      <button type="submit" className={`${PRIMARY_BUTTON} mt-5 self-end`} disabled={disabled}>
        Save
      </button>
    </form>
  );
}
