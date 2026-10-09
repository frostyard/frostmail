// The settings window (docs/specs/settings-ui.md): the pane tabs and the
// containers that connect the Accounts, Signatures and Sign-In panes to
// maild.
import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { ask } from "@tauri-apps/plugin-dialog";
import { AtSign, KeyRound, PenLine, X } from "lucide-react";
import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";

import { useClient } from "../data/session";
import { useMail } from "../data/stores";
import { AccountForm, type AccountFormValue, type DiscoveryState } from "../features/settings/AccountForm";
import { AccountList } from "../features/settings/AccountList";
import { type IdentityChange, IdentityEditor } from "../features/settings/IdentityEditor";
import { OAuthClientForm } from "../features/settings/OAuthClientForm";
import { ServicesSection } from "../features/settings/ServicesSection";
import type {
  Account,
  Client,
  Identity,
  OAuthClient,
  ServerConfig,
  ServiceKind,
  ServiceSettings,
} from "../rpc/gen/api";
import { ErrorCode } from "../rpc/gen/api";
import { RPCError } from "../rpc/transport";
import { openInBrowser } from "./settings";

type Pane = "accounts" | "signatures" | "signin";

const TABS: { pane: Pane; label: string; icon: ReactNode }[] = [
  { pane: "accounts", label: "Accounts", icon: <AtSign size={18} /> },
  { pane: "signatures", label: "Signatures", icon: <PenLine size={18} /> },
  { pane: "signin", label: "Sign-In", icon: <KeyRound size={18} /> },
];

/** SettingsWindow is the settings window's content. */
export function SettingsWindow() {
  const [pane, setPane] = useState<Pane>("accounts");
  useEffect(() => {
    document.title = "Settings";
  }, []);
  const close = () => {
    if (isTauri()) void getCurrentWindow().close();
    else window.close();
  };
  return (
    <div className="flex h-full flex-col bg-window text-primary">
      <div
        data-tauri-drag-region
        className="flex h-[52px] shrink-0 items-center border-b border-separator bg-toolbar px-2"
      >
        <div data-tauri-drag-region className="flex-1" />
        {TABS.map((t) => (
          <button
            key={t.pane}
            type="button"
            aria-pressed={pane === t.pane}
            className={`flex h-11 w-[72px] flex-col items-center justify-center gap-0.5 rounded-md text-[11px] leading-[13px] ${
              pane === t.pane ? "bg-selection-inactive text-primary" : "text-secondary"
            }`}
            onClick={() => setPane(t.pane)}
          >
            {t.icon}
            {t.label}
          </button>
        ))}
        <div data-tauri-drag-region className="flex-1" />
        <button
          type="button"
          aria-label="Close"
          title="Close"
          className="flex h-7 w-7 items-center justify-center rounded-md text-secondary hover:bg-selection-inactive"
          onClick={close}
        >
          <X size={16} />
        </button>
      </div>
      <div className="min-h-0 flex-1">
        {pane === "accounts" && <AccountsPane />}
        {pane === "signatures" && <SignaturesPane />}
        {pane === "signin" && <SignInPane />}
      </div>
    </div>
  );
}

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

/** useRequest runs one request at a time and keeps its error. */
function useRequest() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const run = useCallback(async (f: () => Promise<void>, explain?: (err: unknown) => string | null) => {
    setBusy(true);
    setError(null);
    try {
      await f();
    } catch (err) {
      setError(explain?.(err) ?? message(err));
    } finally {
      setBusy(false);
    }
  }, []);
  return { busy, error, setError, run };
}

const SERVER: Record<"imap" | "smtp", ServerConfig> = {
  imap: { host: "", port: 993, tls: "tls", username: "" },
  smtp: { host: "", port: 465, tls: "tls", username: "" },
};

/** emptyAccount is a new account's form: read-only by default during M4's trial. */
function emptyAccount(): AccountFormValue {
  return {
    kind: "imap",
    email: "",
    displayName: "",
    auth: "password",
    imap: { ...SERVER.imap },
    smtp: { ...SERVER.smtp },
    readOnly: true,
    notify: true,
    password: "",
  };
}

function accountValue(a: Account): AccountFormValue {
  return {
    kind: a.kind,
    email: a.email,
    displayName: a.displayName,
    auth: a.auth,
    imap: { ...a.imap },
    smtp: { ...a.smtp },
    readOnly: a.readOnly,
    notify: a.notify,
    password: "",
  };
}

function noClient(err: unknown): string | null {
  return err instanceof RPCError && err.code === ErrorCode.unavailable
    ? "Set up your Google client under Sign-In first."
    : null;
}

async function signIn(client: Client, id: number): Promise<void> {
  const r = await client.account.authorize({ id });
  await openInBrowser(r.url);
}

async function confirmRemove(email: string): Promise<boolean> {
  const text = `Remove ${email}? Its mail is deleted from this computer, not from the server.`;
  return isTauri() ? ask(text, { title: "Remove Account?", kind: "warning", okLabel: "Remove" }) : window.confirm(text);
}

// Keyed by the selected ID so requests and field state cannot follow the
// user to another account, including one created before the list reloads.
function AccountServices(props: { id: number; onSignIn: () => void; onError: (error: string) => void }) {
  const client = useClient();
  const { id, onError } = props;
  const [services, setServices] = useState<ServiceSettings[]>([]);
  const [busy, setBusy] = useState<ServiceKind | null>(null);
  const [errors, setErrors] = useState<Partial<Record<ServiceKind, string>>>({});
  const active = useRef(true);
  const revision = useRef(0);
  const load = useCallback(async () => {
    const request = ++revision.current;
    try {
      const result = await client.account.services({ id });
      if (active.current && request === revision.current) setServices(result);
    } catch (err) {
      if (active.current && request === revision.current) onError(message(err));
    }
  }, [client, id, onError]);
  useEffect(() => {
    active.current = true;
    void load();
    const off = client.transport.onEvent((event) => {
      if (event.event === "account.changed" && event.data.id === id) void load();
    });
    return () => {
      active.current = false;
      ++revision.current;
      off();
    };
  }, [client, id, load]);
  const toggle = async (service: ServiceKind, enabled: boolean, url?: string) => {
    setBusy(service);
    try {
      await client.account.setService({ id, service, enabled, ...(url === undefined ? {} : { url }) });
      if (!active.current) return;
      setErrors((current) => {
        const next = { ...current };
        delete next[service];
        return next;
      });
      await load();
    } catch (err) {
      if (active.current) setErrors((current) => ({ ...current, [service]: message(err) }));
    } finally {
      if (active.current) setBusy((current) => (current === service ? null : current));
    }
  };
  return (
    <ServicesSection
      services={services}
      busy={busy}
      errors={errors}
      now={new Date()}
      onToggle={(service, enabled, url) => void toggle(service, enabled, url)}
      onSignIn={props.onSignIn}
    />
  );
}

function AccountsPane() {
  const client = useClient();
  const accounts = useMail((s) => s.accounts);
  const [selected, setSelected] = useState<number | "new" | null>(null);
  const [value, setValue] = useState<AccountFormValue>(emptyAccount);
  const [discovery, setDiscovery] = useState<DiscoveryState>({ kind: "idle" });
  const { busy, error, setError, run } = useRequest();
  const account = accounts.find((a) => a.id === selected);

  const select = useCallback(
    (next: number | "new") => {
      setSelected(next);
      setError(null);
      setDiscovery({ kind: "idle" });
      const a = accounts.find((x) => x.id === next);
      setValue(a ? accountValue(a) : emptyAccount());
    },
    [accounts, setError],
  );
  // A selected account that leaves the list was removed, and the form
  // moves to the first. One the list has not shown yet stays selected: a
  // new account's create returns before the list reloads on maild's event.
  const listed = useRef(new Set<number>());
  useEffect(() => {
    for (const a of accounts) listed.current.add(a.id);
    const removed =
      typeof selected === "number" && listed.current.has(selected) && !accounts.some((a) => a.id === selected);
    if (selected === null || removed) select(accounts[0]?.id ?? "new");
  }, [accounts, selected, select]);

  const discover = () =>
    run(async () => {
      setDiscovery({ kind: "finding" });
      try {
        const email = value.email.trim();
        const d = await client.account.discover({ email });
        const withUser = (s: ServerConfig | undefined, cur: ServerConfig) =>
          s ? { ...s, username: s.username === "" ? email : s.username } : cur;
        setValue((v) => ({
          ...v,
          kind: d.kind,
          auth: d.auth[0] ?? "password",
          imap: withUser(d.imap, v.imap),
          smtp: withUser(d.smtp, v.smtp),
        }));
        setDiscovery({ kind: "found", source: d.source });
      } catch (err) {
        setDiscovery({ kind: "failed", message: message(err) });
      }
    });

  const submit = () => {
    if (selected === "new") {
      void run(async () => {
        const a = await client.account.create({
          kind: value.kind,
          email: value.email.trim(),
          displayName: value.displayName,
          auth: value.auth,
          ...(value.imap.host.trim() === "" ? {} : { imap: value.imap }),
          ...(value.smtp.host.trim() === "" ? {} : { smtp: value.smtp }),
          readOnly: value.readOnly,
          notify: value.notify,
        });
        setSelected(a.id);
        setValue(accountValue(a));
        if (value.auth === "oauth2") await signIn(client, a.id);
        else if (value.password !== "") await client.account.setPassword({ id: a.id, password: value.password });
      }, noClient);
    } else if (account) {
      void run(async () => {
        const a = await client.account.update({
          id: account.id,
          displayName: value.displayName,
          imap: value.imap,
          smtp: value.smtp,
          readOnly: value.readOnly,
          notify: value.notify,
        });
        if (value.password !== "") await client.account.setPassword({ id: a.id, password: value.password });
        setValue(accountValue(a));
      });
    }
  };

  const remove = (id: number) => {
    const a = accounts.find((x) => x.id === id);
    if (!a) return;
    void run(async () => {
      if (await confirmRemove(a.email)) await client.account.delete({ id });
    });
  };

  const authorize = () => {
    if (typeof selected === "number") void run(() => signIn(client, selected), noClient);
  };

  return (
    <div className="flex h-full">
      <AccountList
        accounts={accounts}
        selected={selected}
        onSelect={select}
        onAdd={() => select("new")}
        onRemove={remove}
      />
      {selected !== null && (
        <AccountForm
          mode={selected === "new" ? "add" : "edit"}
          value={value}
          onChange={setValue}
          discovery={discovery}
          onDiscover={() => void discover()}
          signedIn={account?.signedIn ?? false}
          onSignIn={authorize}
          services={
            typeof selected === "number" && (
              <AccountServices key={selected} id={selected} onSignIn={authorize} onError={setError} />
            )
          }
          busy={busy}
          error={error}
          onSubmit={submit}
          onCancel={() => select(accounts[0]?.id ?? "new")}
        />
      )}
    </div>
  );
}

function SignaturesPane() {
  const client = useClient();
  const accounts = useMail((s) => s.accounts);
  const [identities, setIdentities] = useState<Identity[]>([]);
  const { busy, error, run } = useRequest();
  // The identities of the accounts shown, reloaded when accounts change.
  const load = useCallback(async () => {
    const list = await client.identity.list({});
    setIdentities(list.filter((i) => accounts.some((a) => a.id === i.accountId)));
  }, [client, accounts]);
  useEffect(() => {
    void load().catch(() => {});
  }, [load]);
  const save = (id: number, change: IdentityChange) =>
    void run(async () => {
      await client.identity.update({ id, ...change });
      await load();
    });
  return <IdentityEditor accounts={accounts} identities={identities} busy={busy} error={error} onSave={save} />;
}

function SignInPane() {
  const client = useClient();
  const [stored, setStored] = useState<OAuthClient | null>(null);
  const { busy, error, setError, run } = useRequest();
  useEffect(() => {
    client.oauth.getClient({ provider: "google" }).then(setStored, (err: unknown) => {
      if (!(err instanceof RPCError && err.code === ErrorCode.notFound)) setError(message(err));
    });
  }, [client, setError]);
  const save = (clientId: string, clientSecret: string) =>
    void run(async () => {
      setStored(
        await client.oauth.setClient({
          provider: "google",
          clientId,
          ...(clientSecret === "" ? {} : { clientSecret }),
        }),
      );
    });
  return <OAuthClientForm client={stored} busy={busy} error={error} onSave={save} />;
}
