// The account list of the settings window's Accounts pane
// (docs/specs/settings-ui.md, AccountList). Task T-0055 implements it; the
// stub draws nothing.
import type { Account } from "../../rpc/gen/api";

/** AccountListProps are the account list's inputs. */
export interface AccountListProps {
  accounts: Account[];
  /** The selected account's ID, "new" while one is being added, or null. */
  selected: number | "new" | null;
  onSelect: (id: number) => void;
  onAdd: () => void;
  onRemove: (id: number) => void;
}

/** AccountList lists the accounts and offers to add or remove one. */
export function AccountList(_props: AccountListProps) {
  return null;
}
