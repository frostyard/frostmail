// The settings window (docs/specs/settings-ui.md): the pane tabs and the
// containers that connect the General, Accounts, Signatures, Rules and Sign-In
// panes to maild.
import { invoke, isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { ask } from "@tauri-apps/plugin-dialog";
import { AtSign, KeyRound, ListFilter, PenLine, Server, Settings2, X } from "lucide-react";
import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";

import { useClient } from "../data/session";
import { useMail } from "../data/stores";
import { newRuleDraft, type RuleDraft, RuleSheet } from "../features/organize/RuleSheet";
import {
  AccountForm,
  type AccountFormValue,
  type DiscoveryState,
  defaultServices,
} from "../features/settings/AccountForm";
import { AccountList } from "../features/settings/AccountList";
import { GeneralPane } from "../features/settings/GeneralPane";
import { type IdentityChange, IdentityEditor } from "../features/settings/IdentityEditor";
import { OAuthClientForm } from "../features/settings/OAuthClientForm";
import { RulesPane } from "../features/settings/RulesPane";
import { ServerPane } from "../features/settings/ServerPane";
import { ServicesSection } from "../features/settings/ServicesSection";
import { closeFrameWindow } from "../lodge/frameWindow";
import type {
  Account,
  Client,
  Identity,
  OAuthClient,
  Rule,
  ServerConfig,
  ServiceKind,
  ServiceSettings,
  SettingsSetParams,
} from "../rpc/gen/api";
import { ErrorCode } from "../rpc/gen/api";
import { RPCError } from "../rpc/transport";
import { browserNotifications } from "./announce";
import { openInBrowser } from "./settings";

type Pane = "general" | "accounts" | "signatures" | "rules" | "signin" | "server";

const TABS: { pane: Pane; label: string; icon: ReactNode }[] = [
  { pane: "general", label: "General", icon: <Settings2 size={18} /> },
  { pane: "accounts", label: "Accounts", icon: <AtSign size={18} /> },
  { pane: "signatures", label: "Signatures", icon: <PenLine size={18} /> },
  { pane: "rules", label: "Rules", icon: <ListFilter size={18} /> },
  { pane: "signin", label: "Sign-In", icon: <KeyRound size={18} /> },
  // Which maild the desktop app asks: not in a browser, which reaches the
  // maild that served it.
  ...(isTauri() ? [{ pane: "server" as const, label: "Server", icon: <Server size={18} /> }] : []),
];

/** SettingsWindow is the settings window's content. */
export function SettingsWindow() {
  const [pane, setPane] = useState<Pane>("accounts");
  useEffect(() => {
    document.title = "Settings";
  }, []);
  const close = () => {
    if (closeFrameWindow()) return;
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
        {pane === "general" && <GeneralPaneContainer />}
        {pane === "accounts" && <AccountsPane />}
        {pane === "signatures" && <SignaturesPane />}
        {pane === "rules" && <RulesPaneContainer />}
        {pane === "signin" && <SignInPane />}
        {pane === "server" && <ServerPaneContainer />}
      </div>
    </div>
  );
}

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

// A browser shows notifications once the user allows them (announce.ts).
function useBrowserNotifications() {
  const [permission, setPermission] = useState(() => (browserNotifications() ? Notification.permission : null));
  if (permission === null) return undefined;
  return {
    permission,
    onAllow: () => {
      void Notification.requestPermission().then(setPermission);
    },
  };
}

function GeneralPaneContainer() {
  const client = useClient();
  const browser = useBrowserNotifications();
  const settings = useMail((s) => s.settings);
  const smarts = useMail((s) => s.smarts);
  const blocked = useMail((s) => s.blocked);
  const [error, setError] = useState<string>();
  const change = async (params: SettingsSetParams) => {
    try {
      await client.settings.set(params);
      setError(undefined);
    } catch (err) {
      setError(message(err));
    }
  };
  const unblock = async (address: string) => {
    try {
      await client.block.remove({ addresses: [address] });
      setError(undefined);
    } catch (err) {
      setError(message(err));
    }
  };
  return (
    <GeneralPane
      settings={settings}
      smarts={smarts}
      blocked={blocked}
      error={error}
      onChange={(params) => void change(params)}
      onUnblock={(address) => void unblock(address)}
      browser={browser}
    />
  );
}

type RuleSheetState = { id?: number; initial: RuleDraft };

function RuleSheetContainer({
  state,
  onClose,
  onSaved,
}: {
  state: RuleSheetState;
  onClose: () => void;
  onSaved: () => void;
}) {
  const client = useClient();
  const { accounts, mailboxes, settings } = useMail();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const date = new Date();
  const today = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
  const save = async ({ name, conditions, actions }: RuleDraft) => {
    if (busy) return;
    setBusy(true);
    setError(undefined);
    try {
      if (state.id === undefined) await client.rule.create({ name, conditions, actions });
      else await client.rule.update({ id: state.id, name, conditions, actions });
      onSaved();
      onClose();
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <RuleSheet
      title={state.id === undefined ? "New Rule" : "Edit Rule"}
      initial={state.initial}
      accounts={accounts}
      mailboxes={mailboxes}
      flagNames={settings?.flagNames}
      today={today}
      busy={busy}
      error={error}
      onSave={(draft) => void save(draft)}
      onCancel={onClose}
    />
  );
}

function RulesPaneContainer() {
  const client = useClient();
  const rules = useMail((s) => s.rules);
  const [error, setError] = useState<string>();
  const [sheet, setSheet] = useState<RuleSheetState | null>(null);
  const change = async (request: () => Promise<unknown>) => {
    try {
      const result = await request();
      if (result !== false) setError(undefined);
    } catch (err) {
      setError(message(err));
    }
  };
  const duplicate = (rule: Rule) =>
    change(async () => {
      const copy = await client.rule.create({
        name: `${rule.name} Copy`,
        conditions: rule.conditions,
        actions: rule.actions,
        enabled: rule.enabled,
      });
      await client.rule.move({ id: copy.id, position: rule.position + 1 });
    });
  const remove = (rule: Rule) =>
    change(async () => {
      const text = `Remove the rule "${rule.name}"?`;
      const confirmed = isTauri()
        ? await ask(text, { title: "Remove Rule?", kind: "warning", okLabel: "Remove" })
        : window.confirm(text);
      if (!confirmed) return false;
      await client.rule.delete({ id: rule.id });
    });
  return (
    <>
      <RulesPane
        rules={rules}
        error={error}
        onToggle={(rule, enabled) => void change(() => client.rule.update({ id: rule.id, enabled }))}
        onAdd={() => setSheet({ initial: newRuleDraft(rules.length + 1) })}
        onEdit={(rule) =>
          setSheet({ id: rule.id, initial: { name: rule.name, conditions: rule.conditions, actions: rule.actions } })
        }
        onDuplicate={(rule) => void duplicate(rule)}
        onRemove={(rule) => void remove(rule)}
        onMove={(rule, position) => void change(() => client.rule.move({ id: rule.id, position }))}
      />
      {sheet && (
        <RuleSheetContainer
          state={sheet}
          onSaved={() => setError(undefined)}
          onClose={() => setSheet((current) => (current === sheet ? null : current))}
        />
      )}
    </>
  );
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

/** emptyAccount is a new account's form: writable, since M5 (parity P-952). */
function emptyAccount(): AccountFormValue {
  return {
    kind: "imap",
    email: "",
    displayName: "",
    auth: "password",
    imap: { ...SERVER.imap },
    smtp: { ...SERVER.smtp },
    readOnly: false,
    notify: true,
    password: "",
    services: [],
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

/** SIGN_IN_WAIT_MS is how long maild waits for a sign-in's return (internal/oauth's FlowTimeout). */
const SIGN_IN_WAIT_MS = 5 * 60 * 1000;

async function signIn(client: Client, id: number): Promise<void> {
  const r = await client.account.authorize({ id });
  await openInBrowser(r.url);
}

async function enableServices(client: Client, id: number, services: ServiceKind[]): Promise<string | null> {
  const names: Record<ServiceKind, string> = { contacts: "Contacts", calendar: "Calendars", tasks: "Tasks" };
  let error: string | null = null;
  for (const service of services) {
    try {
      await client.account.setService({ id, service, enabled: true });
    } catch (err) {
      error ??= `Could not turn on ${names[service]}: ${message(err)}`;
    }
  }
  return error;
}

async function confirmRemove(email: string): Promise<boolean> {
  const text = `Remove ${email}? Its mail is deleted from this computer, not from the server.`;
  return isTauri() ? ask(text, { title: "Remove Account?", kind: "warning", okLabel: "Remove" }) : window.confirm(text);
}

// Keyed by the selected ID so requests and field state cannot follow the
// user to another account, including one created before the list reloads.
function AccountServices(props: {
  id: number;
  provider: string;
  onSignIn: () => void;
  onError: (error: string) => void;
}) {
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
      provider={props.provider}
    />
  );
}

function AccountsPane() {
  const client = useClient();
  const accounts = useMail((s) => s.accounts);
  const [selected, setSelected] = useState<number | "new" | null>(null);
  const [value, setValue] = useState<AccountFormValue>(emptyAccount);
  const [discovery, setDiscovery] = useState<DiscoveryState>({ kind: "idle" });
  // The sign-in this window started, while it waits: its account, and
  // whether that account was signed in before. Only that sign-in can finish
  // from a pasted address (ADR-0043).
  const [waiting, setWaiting] = useState<{ id: number; wasSignedIn: boolean } | null>(null);
  const { busy, error, setError, run } = useRequest();
  const account = accounts.find((a) => a.id === selected);

  const select = useCallback(
    (next: number | "new") => {
      setSelected(next);
      setWaiting((current) => (current?.id === next ? current : null));
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

  // A sign-in that finished elsewhere hides the field: an account that was
  // not signed in now says it is. Signing a signed-in account in again shows
  // no such change, so the field also goes when maild's sign-in expires.
  useEffect(() => {
    if (waiting && !waiting.wasSignedIn && accounts.some((a) => a.id === waiting.id && a.signedIn)) setWaiting(null);
  }, [accounts, waiting]);
  useEffect(() => {
    if (!waiting) return;
    const timer = setTimeout(() => setWaiting(null), SIGN_IN_WAIT_MS);
    return () => clearTimeout(timer);
  }, [waiting]);

  const startSignIn = useCallback(
    async (id: number) => {
      const wasSignedIn = accounts.find((a) => a.id === id)?.signedIn ?? false;
      setWaiting({ id, wasSignedIn });
      try {
        await signIn(client, id);
      } catch (err) {
        setWaiting(null);
        throw err;
      }
    },
    [accounts, client],
  );

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
          services: defaultServices(d.kind),
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
        if (value.auth === "password" && value.password !== "")
          await client.account.setPassword({ id: a.id, password: value.password });
        const serviceError = await enableServices(client, a.id, value.services ?? []);
        if (value.auth === "oauth2") await startSignIn(a.id);
        if (serviceError) throw new Error(serviceError);
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
    if (typeof selected === "number") void run(() => startSignIn(selected), noClient);
  };

  // finishSignIn ends the waiting sign-in with the address of the page the
  // browser could not open. Its errors show like every other request's.
  const finishSignIn = (url: string) =>
    run(async () => {
      await client.account.finishSignIn({ url });
      setWaiting(null);
    });

  return (
    <div className="flex h-full narrow:flex-col">
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
          signInWaiting={typeof selected === "number" && waiting?.id === selected}
          onFinishSignIn={finishSignIn}
          services={
            typeof selected === "number" && (
              <AccountServices
                key={selected}
                id={selected}
                provider={account?.kind === "microsoft" ? "Microsoft" : "Google"}
                onSignIn={authorize}
                onError={setError}
              />
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
  const add = (accountId: number, email: string, name: string) =>
    void run(async () => {
      await client.identity.create({ accountId, email, ...(name === "" ? {} : { name }) });
      await load();
    });
  const remove = (id: number) => {
    const identity = identities.find((i) => i.id === id);
    const account = accounts.find((a) => a.id === identity?.accountId);
    if (!identity || !account) return;
    void run(async () => {
      const text = `Remove ${identity.email}? Drafts from it will be sent from ${account.email}.`;
      const confirmed = isTauri()
        ? await ask(text, { title: "Remove Address?", kind: "warning", okLabel: "Remove" })
        : window.confirm(text);
      if (!confirmed) return;
      await client.identity.delete({ id });
      await load();
    });
  };
  return (
    <IdentityEditor
      accounts={accounts}
      identities={identities}
      busy={busy}
      error={error}
      onSave={save}
      onAdd={add}
      onRemove={remove}
    />
  );
}

function ServerPaneContainer() {
  const [lodge, setLodge] = useState<string | null>(null);
  const { busy, error, setError, run } = useRequest();
  useEffect(() => {
    invoke<{ lodge?: string }>("app_config_get").then(
      (c) => setLodge(c.lodge ?? null),
      (err: unknown) => setError(message(err)),
    );
  }, [setError]);
  const save = (next: string | null) =>
    void run(async () => {
      await invoke("app_config_set", { config: next === null ? {} : { lodge: next } });
      setLodge(next);
    });
  return <ServerPane lodge={lodge} busy={busy} error={error} onSave={save} />;
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
