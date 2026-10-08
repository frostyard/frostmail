// The account list of the settings window's Accounts pane
// (docs/specs/settings-ui.md, AccountList): a row per account with its name
// and status, a row for the account being added, and a bar to add or remove.
import { Minus, Plus } from "lucide-react";

import type { Account } from "../../rpc/gen/api";
import { KIND_LABEL } from "./labels";

/** AccountListProps are the account list's inputs. */
export interface AccountListProps {
  accounts: Account[];
  /** The selected account's ID, "new" while one is being added, or null. */
  selected: number | "new" | null;
  onSelect: (id: number) => void;
  onAdd: () => void;
  onRemove: (id: number) => void;
}

const ROW = "flex w-full flex-col px-3 py-1.5 text-left";
const NAME = "truncate text-[13px] leading-[18px] font-semibold";
const STATUS = "truncate text-[12px] leading-4";
const ICON_BUTTON =
  "flex h-6 w-6 items-center justify-center rounded text-secondary hover:bg-selection-inactive disabled:opacity-40";

function nameOf(account: Account): string {
  return account.displayName.trim() === "" ? account.email : account.displayName;
}

function statusOf(account: Account): { text: string; className: string } {
  if (!account.signedIn) {
    return { text: "Sign in again", className: `${STATUS} text-flag-1` };
  }
  const text = account.readOnly ? `${KIND_LABEL[account.kind]} · Read-only` : KIND_LABEL[account.kind];
  return { text, className: `${STATUS} text-secondary` };
}

/** AccountList lists the accounts and offers to add or remove one. */
export function AccountList({ accounts, selected, onSelect, onAdd, onRemove }: AccountListProps) {
  const selectedId = typeof selected === "number" ? selected : null;
  return (
    <nav aria-label="Accounts" className="flex h-full w-[220px] shrink-0 flex-col border-r border-separator bg-sidebar">
      <ul className="flex-1 overflow-y-auto py-2">
        {accounts.map((account) => {
          const isSelected = account.id === selectedId;
          const status = statusOf(account);
          return (
            <li key={account.id}>
              <button
                type="button"
                aria-current={isSelected ? "true" : undefined}
                className={isSelected ? `${ROW} bg-selection-sidebar` : ROW}
                onClick={() => onSelect(account.id)}
              >
                <span className={NAME}>{nameOf(account)}</span>
                <span className={status.className}>{status.text}</span>
              </button>
            </li>
          );
        })}
        {selected === "new" && (
          <li>
            <div aria-current="true" className="flex w-full flex-col bg-selection-sidebar px-3 py-1.5">
              <span className={NAME}>New Account</span>
            </div>
          </li>
        )}
      </ul>
      <div className="flex h-8 shrink-0 items-center gap-1 border-t border-separator px-2">
        <button
          type="button"
          aria-label="Add Account"
          className={ICON_BUTTON}
          disabled={selected === "new"}
          onClick={onAdd}
        >
          <Plus size={14} />
        </button>
        <button
          type="button"
          aria-label="Remove Account"
          className={ICON_BUTTON}
          disabled={selectedId === null}
          onClick={() => {
            if (selectedId !== null) onRemove(selectedId);
          }}
        >
          <Minus size={14} />
        </button>
      </div>
    </nav>
  );
}
