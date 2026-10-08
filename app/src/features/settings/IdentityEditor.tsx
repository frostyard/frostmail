// The Signatures pane of the settings window (docs/specs/settings-ui.md,
// IdentityEditor): it lists every account's identities and edits the
// selected one's name, Reply-To and signature as plain text.
import { type FormEvent, Fragment, useEffect, useState } from "react";

import { signatureHtml, signatureText } from "../../lib/signature";
import type { Account, Identity } from "../../rpc/gen/api";
import { ALERT, FIELD, GRID, LABEL, PRIMARY_BUTTON } from "./labels";

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
export function IdentityEditor({ accounts, identities, busy, error, onSave }: IdentityEditorProps) {
  const groups = accounts
    .map((account) => ({ account, identities: identities.filter((i) => i.accountId === account.id) }))
    .filter((group) => group.identities.length > 0);
  const all = groups.flatMap((group) => group.identities);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const selected = all.find((i) => i.id === selectedId) ?? all[0] ?? null;

  if (!selected) {
    return <p className="px-6 py-5 text-[13px] leading-[18px] text-secondary">Add an account to edit its signature.</p>;
  }

  return (
    <div className="flex h-full">
      <ul
        aria-label="Identities"
        className="w-[220px] shrink-0 overflow-y-auto border-r border-separator bg-sidebar py-2"
      >
        {groups.map((group) => (
          <Fragment key={group.account.id}>
            <li className="px-3 pt-2 pb-1 text-sidebar-section text-secondary">{group.account.email}</li>
            {group.identities.map((identity) => (
              <li key={identity.id}>
                <button
                  type="button"
                  aria-current={identity.id === selected.id ? "true" : undefined}
                  className={`flex w-full flex-col px-3 py-1.5 text-left ${
                    identity.id === selected.id ? "bg-selection-sidebar" : ""
                  }`}
                  onClick={() => setSelectedId(identity.id)}
                >
                  <span className="truncate text-[13px] leading-[18px] font-semibold">
                    {identity.name.trim() === "" ? identity.email : identity.name}
                  </span>
                  <span className="truncate text-[12px] leading-4 text-secondary">{identity.email}</span>
                </button>
              </li>
            ))}
          </Fragment>
        ))}
      </ul>
      <IdentityForm identity={selected} busy={busy} error={error} onSave={onSave} />
    </div>
  );
}

/** IdentityForm edits the selected identity's name, Reply-To and signature. */
function IdentityForm({
  identity,
  busy,
  error,
  onSave,
}: {
  identity: Identity;
  busy: boolean;
  error: string | null;
  onSave: (id: number, change: IdentityChange) => void;
}) {
  const storedText = signatureText(identity.signatureHtml);
  const [name, setName] = useState(identity.name);
  const [replyTo, setReplyTo] = useState(identity.replyTo);
  const [text, setText] = useState(storedText);

  useEffect(() => {
    setName(identity.name);
    setReplyTo(identity.replyTo);
    setText(signatureText(identity.signatureHtml));
  }, [identity.name, identity.replyTo, identity.signatureHtml]);

  const dirty = name !== identity.name || replyTo !== identity.replyTo || text !== storedText;

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (busy || !dirty) return;
    onSave(identity.id, { name, replyTo, signatureHtml: signatureHtml(text) });
  };

  return (
    <form className="flex flex-1 flex-col px-6 py-5" onSubmit={submit}>
      <div className={GRID}>
        <label htmlFor="identity-name" className={LABEL}>
          Name:
        </label>
        <input
          id="identity-name"
          type="text"
          className={FIELD}
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <span className={LABEL}>Email:</span>
        <span className="text-[13px] leading-[18px]">{identity.email}</span>
        <label htmlFor="identity-reply-to" className={LABEL}>
          Reply-To:
        </label>
        <input
          id="identity-reply-to"
          type="email"
          className={FIELD}
          placeholder="None"
          value={replyTo}
          onChange={(e) => setReplyTo(e.target.value)}
        />
        <label htmlFor="identity-signature" className={`${LABEL} self-start pt-1`}>
          Signature:
        </label>
        <textarea
          id="identity-signature"
          rows={8}
          className="min-h-[160px] rounded-md border border-separator bg-window p-2 text-[13px] leading-[18px] focus:outline-none focus:ring-2 focus:ring-focus"
          value={text}
          onChange={(e) => setText(e.target.value)}
        />
      </div>
      {error !== null && error !== "" && (
        <p role="alert" className={ALERT}>
          {error}
        </p>
      )}
      <div className="mt-auto flex justify-end pt-5">
        <button type="submit" className={PRIMARY_BUTTON} disabled={busy || !dirty}>
          Save
        </button>
      </div>
    </form>
  );
}
