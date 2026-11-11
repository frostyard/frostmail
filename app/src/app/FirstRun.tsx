// The main window with no account (docs/specs/ui.md, First run): a welcome in
// place of the modules, which adds an account.
import { PRIMARY_BUTTON } from "../features/settings/labels";
import { openSettings } from "./settings";

/** FirstRun is the welcome a store with no account shows in place of the modules. */
export function FirstRun() {
  const addAccount = () => {
    void openSettings("accounts").catch((err: unknown) => console.warn("settings", err));
  };
  return (
    <section aria-label="Welcome" className="flex min-h-0 flex-1 items-center justify-center bg-window px-6">
      <div className="w-full max-w-[420px]">
        <h1 className="text-[17px] leading-[22px] font-semibold">Welcome to Frostmail</h1>
        <p className="mt-2 text-[13px] leading-[18px] text-secondary">Add a mail account to start.</p>
        <div className="mt-5 flex items-center gap-2">
          <button type="button" className={PRIMARY_BUTTON} onClick={addAccount}>
            Add Account…
          </button>
        </div>
      </div>
    </section>
  );
}
