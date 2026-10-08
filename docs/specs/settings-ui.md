# Spec: the settings window (M4)

The look and behavior of Frostmail's settings window: accounts (adding one
with discovery and Google sign-in, editing, removing), signatures and
identities, and the Google OAuth client. It binds the presentational
components in `app/src/features/settings/`, the containers that wire them
to maild, and the app tests. Tokens, type, button styles and the
conventions for measurements are those of the reader UI ([ui.md](ui.md));
this spec adds no tokens.

## Window

```
┌──────────────────────────────────────────────────────────────────┐
│          [@ Accounts] [✎ Signatures] [🔑 Sign-In]          ✕   │  toolbar 52
├─────────────────┬────────────────────────────────────────────────┤
│ Ann Example     │ Ann Example                                    │
│ Gmail           │   Email Address: ann@gmail.com                 │
│▌iCloud          │       Full Name: Ann Example                   │
│ iCloud · Read…  │    Account Type: Gmail ▾                       │
│                 │         Sign In: Google sign-in ▾              │
│                 │          Google: Signed in  [Sign In Again…]   │
│                 │ Incoming Mail (IMAP) …                         │
│                 │ Outgoing Mail (SMTP) …                         │
│ + −             │ ☐ Read only  ☑ Notify               [Save]     │
└─────────────────┴────────────────────────────────────────────────┘
```

- One window, label `settings`, 760 × 560, minimum 640 × 440, no system
  title bar. Opening it again (Ctrl+, in the main window, or the main
  toolbar's Settings button) shows and focuses the open one. It is created
  hidden and shown after its first render, like compose windows.
- **Toolbar (52, `--bg-toolbar`, 1px `--separator` bottom, drag region):**
  the pane tabs centered, the Close button (`X`, as in the main toolbar) at
  the right. A tab is a `button` 72 × 44, rounded 6, an 18px Lucide icon
  over its 11/13 label, `--text-secondary`; the open pane's tab is
  `--selection-inactive` filled with `--text-primary` and
  `aria-pressed="true"`. Tabs: Accounts (`AtSign`), Signatures (`PenLine`),
  Sign-In (`KeyRound`).
- The pane fills the rest of the window.

## Shared pieces

`app/src/features/settings/labels.ts` holds what the panes share:
`KIND_LABEL` (imap "IMAP", gmail "Gmail", microsoft "Microsoft", icloud
"iCloud"), `TLS_MODES` and `TLS_LABEL`, and the classes named below:
`GRID` (labeled rows), `LABEL`, `FIELD` (text inputs and selects),
`BUTTON` (secondary), `PRIMARY_BUTTON` and `ALERT`. Components use these
constants rather than repeating the classes.

## Accounts pane

A list on the left (`AccountList`) and the selected account's form on the
right (`AccountForm`). With no account, the pane starts adding one.

### `AccountList`

```ts
interface AccountListProps {
  accounts: Account[];
  selected: number | "new" | null; // an account ID, or "new" while adding
  onSelect: (id: number) => void;
  onAdd: () => void;
  onRemove: (id: number) => void;
}
```

- A `nav` with `aria-label="Accounts"`: `flex h-full w-[220px] shrink-0
  flex-col border-r border-separator bg-sidebar`.
- A `ul` (`flex-1 overflow-y-auto py-2`) with one `li` per account, in the
  given order, holding a `button type="button"` (`flex w-full flex-col
  px-3 py-1.5 text-left`, plus `bg-selection-sidebar` when selected) with
  `aria-current="true"` when selected; clicking calls `onSelect(id)`. The
  button holds two `span`s:
  1. the display name, or the email when the trimmed name is empty
     (`truncate text-[13px] leading-[18px] font-semibold`);
  2. the status (`truncate text-[12px] leading-4`, plus `text-secondary`,
     or `text-flag-1` when not signed in): `Sign in again` when
     `signedIn` is false, else the kind's label (`KIND_LABEL`) followed by
     ` · Read-only` for a read-only account.
- While `selected` is `"new"`, a last `li` holds a `div` (`flex w-full
  flex-col bg-selection-sidebar px-3 py-1.5`) with `aria-current="true"`
  and a `span` "New Account" (`truncate text-[13px] leading-[18px]
  font-semibold`).
- Below the list, a bar (`flex h-8 shrink-0 items-center gap-1 border-t
  border-separator px-2`) with two icon buttons (`button type="button"`,
  `flex h-6 w-6 items-center justify-center rounded text-secondary
  hover:bg-selection-inactive disabled:opacity-40`, 14px icons):
  - `aria-label="Add Account"` (`Plus`): calls `onAdd`; disabled while
    `selected` is `"new"`;
  - `aria-label="Remove Account"` (`Minus`): calls `onRemove(selected)`;
    disabled unless an account is selected.

### `ServerFields`

```ts
interface ServerFieldsProps {
  id: "imap" | "smtp"; // the inputs' ID prefix and the default ports
  legend: string;      // "Incoming Mail (IMAP)" or "Outgoing Mail (SMTP)"
  value: ServerConfig;
  onChange: (v: ServerConfig) => void;
  disabled?: boolean;
}
```

- A `fieldset` (`grid grid-cols-[128px_1fr] items-center gap-x-3
  gap-y-2`) with `disabled` when `disabled`; its `legend` (`mb-2
  text-[13px] leading-[18px] font-semibold`) is the legend.
- Four rows, each a `label` (`text-right text-[13px] leading-[18px]
  text-secondary`, `htmlFor` its control) and its control:

| Label | Control | `id` | Value and change |
| --- | --- | --- | --- |
| `Server:` | `input type="text"` | `<id>-host` | `host` |
| `Port:` | `input type="number"`, `min=1`, `max=65535`, plus `w-24` | `<id>-port` | `port`, shown empty when 0; a change sets `Number.parseInt(text, 10)`, or 0 when that is not a number |
| `Security:` | `select` | `<id>-tls` | `tls`; options in order `tls` "TLS", `starttls` "STARTTLS", `insecure` "None (insecure)" |
| `User Name:` | `input type="text"` | `<id>-username` | `username` |

- Text inputs and the select share `h-7 rounded-md border border-separator
  bg-window px-2 text-[13px] leading-[18px] focus:outline-none
  focus:ring-2 focus:ring-focus`; text inputs also have
  `autoComplete="off"` and `spellCheck={false}`.
- **Default ports:** `defaultPort(id, tls)` is 993 / 143 / 143 for IMAP
  over `tls` / `starttls` / `insecure`, and 465 / 587 / 25 for SMTP.
  Changing Security also changes the port to the new mode's default when
  the port is 0 or the old mode's default; any other port stays.
- Every change calls `onChange` with a new object; the value passed in is
  never mutated.

### `AccountForm`

```ts
interface AccountFormValue {
  kind: AccountKind;
  email: string;
  displayName: string;
  auth: AuthKind;
  imap: ServerConfig;
  smtp: ServerConfig;
  readOnly: boolean;
  notify: boolean;
  password: string; // a new password; "" keeps the stored one
}
type DiscoveryState =
  | { kind: "idle" }
  | { kind: "finding" }
  | { kind: "found"; source: DiscoverySource }
  | { kind: "failed"; message: string };
interface AccountFormProps {
  mode: "add" | "edit";
  value: AccountFormValue;
  onChange: (v: AccountFormValue) => void;
  discovery: DiscoveryState; // add mode
  onDiscover: () => void;    // add mode
  signedIn: boolean;         // edit mode: Account.signedIn
  onSignIn: () => void;      // edit mode, oauth2: browser sign-in
  busy: boolean;             // a request is running
  error: string | null;      // the last request's error
  onSubmit: () => void;
  onCancel: () => void;      // add mode
}
```

- A `form` (`flex h-full flex-1 flex-col overflow-y-auto px-6 py-5`);
  submitting it prevents the default and calls `onSubmit`.
- An `h2` (`mb-4 text-[15px] leading-5 font-semibold`): "Add Account" in
  add mode; in edit mode the display name, or the email when the trimmed
  name is empty.
- A grid (`grid grid-cols-[128px_1fr] items-center gap-x-3 gap-y-2`) of
  labeled rows, labels and controls styled as in `ServerFields`:

| Label | Control | `id` | Notes |
| --- | --- | --- | --- |
| `Email Address:` | `input type="email"` | `account-email` | `readOnly` in edit mode |
| `Full Name:` | `input type="text"` | `account-name` | `displayName` |
| `Account Type:` | `select` | `account-kind` | options `imap` "IMAP", `gmail` "Gmail", `icloud` "iCloud"; `disabled` in edit mode |
| `Sign In:` | `select` | `account-auth` | options `password` "Password", and `oauth2` "Google sign-in" only when the kind is `gmail`; `disabled` in edit mode |
| `Password:` | `input type="password"`, `autoComplete="new-password"` | `account-password` | only when auth is `password`; placeholder "Unchanged" in edit mode |
| `Google:` | see below | | only when auth is `oauth2`; the label is a `span` |

- **Add mode, after the email row:** a row whose second column holds a
  `button type="button"` "Find Settings" (secondary style) calling
  `onDiscover`, disabled while `busy`, while finding, or when the trimmed
  email does not match `^[^@\s]+@[^@\s]+$`; then a `span` (`text-[12px]
  leading-4`) after it with the discovery state: finding `Looking up
  servers…` (`text-secondary`); found with source `none`: `No settings
  found; enter the servers below.` (`text-secondary`); found otherwise:
  `Found settings for this address.` (`text-secondary`); failed: the
  message (`text-flag-1`); idle: no span.
- **Changing the kind** to anything but `gmail` also sets auth to
  `password`.
- **Password hint:** under the password row, for kind `gmail`: "Use an app
  password from your Google account."; for `icloud`: "Use an app-specific
  password from appleid.apple.com."; a `p` (`col-start-2 text-[12px]
  leading-4 text-secondary`).
- **Google row (oauth2):** in add mode a `p` (`text-[12px] leading-4
  text-secondary`): "After adding the account, sign in with Google in your
  browser."; in edit mode a `span` (`text-[13px] leading-[18px]`): `Signed
  in` (`text-secondary`) or `Not signed in` (`text-flag-1`), then a
  `button type="button"` (secondary style, `ml-3`) `Sign In Again…` when
  signed in, else `Sign In…`, calling `onSignIn`, disabled while `busy`.
- Then, each in a `div` with `mt-5`: `ServerFields id="imap"` "Incoming
  Mail (IMAP)" and `ServerFields id="smtp"` "Outgoing Mail (SMTP)",
  disabled while `busy`.
- Then options (`mt-5 flex flex-col gap-2`), each a `label` (`flex
  items-center gap-2 text-[13px] leading-[18px]`) around an `input
  type="checkbox"` and its text: "Read only: never change anything on the
  server" (`readOnly`), "Notify me about new mail" (`notify`).
- When `error` is set, a `p` with `role="alert"` (`mt-4 text-[12px]
  leading-4 text-flag-1`) shows it.
- Buttons (`mt-auto flex justify-end gap-2 pt-5`): in add mode "Cancel"
  (`button type="button"`, secondary, `onCancel`) then "Add Account"; in
  edit mode "Save". The submit button is `type="submit"` (primary style)
  and disabled while `busy`.
- Button styles: secondary `h-7 rounded-md border border-separator
  bg-window px-3 text-[13px] leading-[18px] disabled:opacity-40`; primary
  `h-7 rounded-md bg-accent px-3 text-[13px] leading-[18px] font-semibold
  text-accent-contrast disabled:opacity-40`.

### Behavior (container)

- **Add:** + selects "New Account" with an empty form (kind `imap`, auth
  `password`, TLS defaults, notify on). Find Settings calls
  `account.discover` and fills kind, the first auth kind, and the servers
  it found; a found username left empty becomes the email. Add Account
  calls `account.create`, then `account.setPassword` (password auth) or
  `account.authorize` and opens its URL in the system browser (oauth2),
  then selects the new account. An `unavailable` error from authorize
  means no Google client is stored: the error says "Set up your Google
  client under Sign-In first."
- **Edit:** Save sends `account.update` with every editable field, then
  `account.setPassword` when a new password was typed. Sign In… calls
  `account.authorize` and opens the URL; `account.changed` updates the
  status.
- **Remove:** asks "Remove <email>? Its mail is deleted from this computer,
  not from the server." and calls `account.delete` when confirmed.
- Errors from maild show in the form's alert; the form keeps its values.

## Signatures pane

### `IdentityEditor`

```ts
interface IdentityChange { name: string; replyTo: string; signatureHtml: string }
interface IdentityEditorProps {
  accounts: Account[];
  identities: Identity[];
  busy: boolean;
  error: string | null;
  onSave: (id: number, change: IdentityChange) => void;
}
```

- A `div` (`flex h-full`) with the list and the form.
- **List:** a `ul` with `aria-label="Identities"` (`w-[220px] shrink-0
  overflow-y-auto border-r border-separator bg-sidebar py-2`). For each
  account in order with at least one identity: an `li` (`px-3 pt-2 pb-1
  text-sidebar-section text-secondary`) with the account's email, then one
  `li` per identity of that account, in the given order, holding a
  `button type="button"` (`flex w-full flex-col px-3 py-1.5 text-left`,
  plus `bg-selection-sidebar` when selected, `aria-current="true"` when
  selected) with the name, or the email when the trimmed name is empty
  (`truncate text-[13px] leading-[18px] font-semibold`), then the email
  (`truncate text-[12px] leading-4 text-secondary`).
- The first identity in list order starts selected; clicking another
  selects it.
- **Form:** a `form` (`flex flex-1 flex-col px-6 py-5`) for the selected
  identity, in the grid of `AccountForm`: `Name:` (`identity-name`),
  `Email:` (a `span`, `text-[13px] leading-[18px]`, with the email; not
  editable), `Reply-To:` (`identity-reply-to`, `input type="email"`,
  placeholder "None"), `Signature:` (`identity-signature`, a `textarea`
  with `rows={8}`, classes `min-h-[160px] rounded-md border
  border-separator bg-window p-2 text-[13px] leading-[18px]
  focus:outline-none focus:ring-2 focus:ring-focus`; the label is
  `self-start pt-1` as well).
- The fields show the selected identity's stored values (the signature
  through `signatureText`) when it is selected and whenever its stored
  values change.
- An alert as in `AccountForm`, then a "Save" submit button (primary
  style, right-aligned, `mt-auto pt-5`), disabled while `busy` or while no
  field differs from the stored values. Saving calls `onSave(id, {name,
  replyTo, signatureHtml: signatureHtml(text)})` with the fields' values
  as typed.
- With no identity, the pane shows only a `p` (`px-6 py-5 text-[13px]
  leading-[18px] text-secondary`): "Add an account to edit its
  signature."

### Signature text

`app/src/lib/signature.ts` converts between the stored HTML and the plain
text the editor shows:

- `signatureHtml(text)`: "" when the text is blank; otherwise one
  `<p>…</p>` per line, with `&`, `<`, `>` and `"` escaped, an empty line
  becoming `<p><br></p>`; trailing empty lines are dropped.
- `signatureText(html)`: the text of each top-level block, one per line,
  with `<br>` inside a block becoming a line break; text outside blocks is
  kept as is.

## Sign-In pane

### `OAuthClientForm`

```ts
interface OAuthClientFormProps {
  client: OAuthClient | null; // the stored Google client, or null
  busy: boolean;
  error: string | null;
  onSave: (clientId: string, clientSecret: string) => void; // "" keeps the secret
}
```

- A `form` with `aria-label="Google client"` (`flex max-w-[560px]
  flex-col px-6 py-5`).
- An `h2` "Google Client" (as in `AccountForm`), then a `p` (`mb-4
  text-[12px] leading-4 text-secondary`): "Gmail sign-in uses your own
  Google OAuth client until Frostmail has a verified one. Create a Desktop
  app client in Google Cloud and enter it here."
- The `AccountForm` grid with `Client ID:` (`oauth-client-id`, text,
  starting as the stored client ID or empty) and `Client Secret:`
  (`oauth-client-secret`, `input type="password"`,
  `autoComplete="off"`, starting empty, placeholder "Stored" when a
  secret is stored).
- A status `p` (`mt-3 text-[12px] leading-4 text-secondary`): "No client
  stored", "Client and secret stored", or "Client stored without a
  secret".
- An alert as in `AccountForm`, then "Save" (primary, submit, `mt-5
  self-end`), disabled while `busy`, while the trimmed client ID is empty,
  or while the trimmed client ID equals the stored one and the secret is
  empty. Saving calls `onSave(trimmed ID, secret)`; afterwards the secret
  field is emptied.

## References

- Rationale: [ADR-0011](../adr/0011-sign-in-and-credentials.md)
- Context: [design/accounts.md](../design/accounts.md),
  [plan 0006](../plans/0006-m4-daily-driver.md)
- Shared conventions: [ui.md](ui.md), [compose-ui.md](compose-ui.md)
