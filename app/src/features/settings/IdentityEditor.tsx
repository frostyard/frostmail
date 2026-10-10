// The Signatures pane of the settings window (docs/specs/settings-ui.md,
// IdentityEditor): it lists every account's identities and edits the
// selected one's name, Reply-To and signature as plain text.
import { Minus, Plus } from "lucide-react";
import { type FormEvent, Fragment, useEffect, useState } from "react";

import { signatureHtml, signatureText } from "../../lib/signature";
import type { Account, Identity } from "../../rpc/gen/api";
import { ALERT, BUTTON, FIELD, GRID, LABEL, PRIMARY_BUTTON } from "./labels";

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
  /** Adds an address to an account; name "" takes the account's. */
  onAdd: (accountId: number, email: string, name: string) => void;
  /** Removes an added address. */
  onRemove: (id: number) => void;
}

/** IdentityEditor lists identities and edits the selected one. */
export function IdentityEditor({ accounts, identities, busy, error, onSave, onAdd, onRemove }: IdentityEditorProps) {
  const groups = accounts
    .map((account) => ({ account, identities: identities.filter((i) => i.accountId === account.id) }))
    .filter((group) => group.identities.length > 0);
  const all = groups.flatMap((group) => group.identities);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const selected = all.find((i) => i.id === selectedId) ?? all[0] ?? null;

  const [adding, setAdding] = useState<number | null>(null);
  const [pending, setPending] = useState<{ accountId: number; email: string; ids: number[] } | null>(null);
  useEffect(() => {
    if (!pending) return;
    const added = identities.find(
      (i) =>
        !pending.ids.includes(i.id) &&
        i.accountId === pending.accountId &&
        i.email.toLowerCase() === pending.email.toLowerCase(),
    );
    if (added) {
      setSelectedId(added.id);
      setAdding(null);
      setPending(null);
    }
  }, [identities, pending]);
  const closeAdd = () => {
    setAdding(null);
    setPending(null);
  };

  if (!selected) {
    return <p className="px-6 py-5 text-[13px] leading-[18px] text-secondary">Add an account to edit its signature.</p>;
  }

  return (
    <div className="flex h-full">
      <div className="flex shrink-0 flex-col">
        <ul
          aria-label="Identities"
          className="w-[220px] flex-1 shrink-0 overflow-y-auto border-r border-separator bg-sidebar py-2"
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
                    onClick={() => {
                      setSelectedId(identity.id);
                      closeAdd();
                    }}
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
        <div className="flex h-8 w-[220px] shrink-0 items-center gap-1 border-t border-r border-separator bg-sidebar px-2">
          <button
            type="button"
            aria-label="Add Address"
            className={ICON_BUTTON}
            disabled={busy || adding !== null}
            onClick={() => setAdding(selected.accountId)}
          >
            <Plus size={14} />
          </button>
          <button
            type="button"
            aria-label="Remove Address"
            className={ICON_BUTTON}
            disabled={busy || adding !== null || selected.isDefault}
            onClick={() => onRemove(selected.id)}
          >
            <Minus size={14} />
          </button>
        </div>
      </div>
      {adding !== null ? (
        <AddAddressForm
          accounts={groups.map((g) => g.account)}
          identities={identities}
          initialAccountId={adding}
          busy={busy}
          error={error}
          onCancel={closeAdd}
          onAdd={(accountId, email, name) => {
            setPending({ accountId, email, ids: identities.map((i) => i.id) });
            onAdd(accountId, email, name);
          }}
        />
      ) : (
        <IdentityForm key={selected.id} identity={selected} busy={busy} error={error} onSave={onSave} />
      )}
    </div>
  );
}

const ICON_BUTTON =
  "flex h-6 w-6 items-center justify-center rounded text-secondary hover:bg-selection-inactive disabled:opacity-40";

function AddAddressForm({
  accounts,
  identities,
  initialAccountId,
  busy,
  error,
  onAdd,
  onCancel,
}: {
  accounts: Account[];
  identities: Identity[];
  initialAccountId: number;
  busy: boolean;
  error: string | null;
  onAdd: IdentityEditorProps["onAdd"];
  onCancel: () => void;
}) {
  const [accountId, setAccountId] = useState(initialAccountId);
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const placeholder = identities.find((i) => i.accountId === accountId && i.isDefault)?.name ?? "";
  const disabled = busy || !/^[^@\s]+@[^@\s]+$/.test(email.trim());
  return (
    <form
      className="flex flex-1 flex-col px-6 py-5"
      onSubmit={(event) => {
        event.preventDefault();
        if (!disabled) onAdd(accountId, email.trim(), name.trim());
      }}
    >
      <h2 className="mb-4 text-[15px] leading-5 font-semibold">Add Address</h2>
      <div className={GRID}>
        <label htmlFor="identity-new-account" className={LABEL}>
          Account:
        </label>
        <select
          id="identity-new-account"
          className={FIELD}
          value={accountId}
          onChange={(event) => setAccountId(Number(event.target.value))}
        >
          {accounts.map((a) => (
            <option key={a.id} value={a.id}>
              {a.email}
            </option>
          ))}
        </select>
        <label htmlFor="identity-new-email" className={LABEL}>
          Email Address:
        </label>
        <input
          id="identity-new-email"
          type="email"
          className={FIELD}
          value={email}
          onChange={(event) => setEmail(event.target.value)}
        />
        <label htmlFor="identity-new-name" className={LABEL}>
          Full Name:
        </label>
        <input
          id="identity-new-name"
          type="text"
          className={FIELD}
          placeholder={placeholder}
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
        <p className="col-start-2 text-[12px] leading-4 text-secondary">
          An address your provider delivers to this account, such as an alias or your own domain. Frostmail sends from
          it and answers invitations to it.
        </p>
      </div>
      {error && (
        <p role="alert" className={ALERT}>
          {error}
        </p>
      )}
      <div className="mt-auto flex justify-end gap-2 pt-5">
        <button type="button" className={BUTTON} onClick={onCancel}>
          Cancel
        </button>
        <button type="submit" className={PRIMARY_BUTTON} disabled={disabled}>
          Add
        </button>
      </div>
    </form>
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
