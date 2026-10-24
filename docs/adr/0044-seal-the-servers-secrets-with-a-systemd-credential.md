# 0044 — On a server, maild seals its secrets file with a key from a systemd credential

- **Status:** Proposed
- **Date:** 2026-10-24

## Context

maild keeps every account's credential outside its database: a password,
a refresh token, or an OAuth client's secret, under the keys
`account/<id>/password`, `account/<id>/refresh-token` and
`oauth/<provider>/client-secret`
([design/accounts.md](../design/accounts.md#secrets)). Where they live
depends on the platform:

- **On a Linux desktop,** in the Secret Service on the session bus
  ([ADR-0011](0011-sign-in-and-credentials.md)). The user's keyring holds
  them, unlocked at sign-in.
- **On Windows,** in `secrets.dpapi`, the same JSON object sealed for the
  user with DPAPI ([ADR-0038](0038-windows-as-a-second-platform.md)).
- **Without either** (CI, the nsl machine), in `secrets.json`: plain text,
  mode 0600, beside the database.

[ADR-0041](0041-maild-runs-as-a-lodge-app.md) runs the owner's maild on
lodge's host as the system user `frostmail`. A system service has no
session bus and no keyring, so `packaging/lodge/frostmail.service` sets
`FROSTMAIL_SECRETS=file`: the secrets are plain text in
`/var/lib/frostmail/secrets.json`, mode 0600, owner `frostmail`, in a
0700 state directory. ADR-0041 left the choice between that file and a
sealed store to [plan 0011](../plans/0011-m8-mail-in-lodge.md), to make
before Phase 7 moves the owner's own accounts to the server.

Facts that shape the choice:

- **maild writes secrets while it runs.** A sign-in stores a refresh
  token, `account.setPassword` a password, and `oauth.setClient` a client
  secret. Microsoft rotates the refresh token on every refresh, and maild
  stores each new one ([ADR-0039](0039-microsoft-accounts-over-imap-smtp-and-graph.md)).
  Any store must take writes from maild, unattended, at any hour.
- **The secrets can be obtained again.** Each is the result of a sign-in or
  of the owner typing it. Losing them all costs one Sign In Again for each
  account, and no mail: the mail stays on the IMAP servers.
- **Who can read them on the server.** A process running as `frostmail`,
  and root, can read the file. Either can also read the database and
  act as the owner through maild, so no store on the same host keeps the
  secrets from them. The difference between stores is what a copy of
  the state directory exposes when it leaves the running host: a backup of
  `/var/lib`, a snapshot of the VM's disk, a disk image read from outside.
- **systemd credentials** (systemd 250 and later). `systemd-creds encrypt`
  seals a small file with AES-256-GCM under a key derived from the host's
  `/var/lib/systemd/credential.secret`, the host's TPM2, or both
  (`--with-key=auto`: TPM2 when the machine has one, and the host key when
  `/var/lib/systemd` is on persistent storage). A unit's
  `LoadCredentialEncrypted=ID` finds the sealed file in
  `/etc/credstore.encrypted/`, unseals it as the service starts, and
  gives it to the service's user only, in `$CREDENTIALS_DIRECTORY`, on
  memory that is not swapped when possible. A credential is read-only to
  the service, and systemd reads it once, at the start.
- **Some sealing exists already.** The file store has seal and unseal hooks,
  which Windows uses for DPAPI, and maild already moves a plain
  `secrets.json` it finds into the sealed store on start, then removes it.

## Decision

- **On a server, the secrets stay in maild's own file store, sealed.**
  maild seals the whole JSON object with AES-256-GCM, with a fresh random
  nonce on each write, as `secrets.sealed` in its data directory, mode
  0600. It writes the file as it writes `secrets.json` today: a temporary
  file, synced, then renamed over the old one.
- **The key is a systemd credential.** It is 32 random bytes, which the
  install seals once with `systemd-creds encrypt --with-key=auto
  --name=frostmail-secrets-key` into
  `/etc/credstore.encrypted/frostmail-secrets-key`.
  `frostmail.service` gains `LoadCredentialEncrypted=frostmail-secrets-key`
  and sets `FROSTMAIL_SECRETS=sealed`. maild reads the key from
  `$CREDENTIALS_DIRECTORY/frostmail-secrets-key`.
- **No silent fallback.** With `FROSTMAIL_SECRETS=sealed`, maild refuses to
  start when the key is missing, is not 32 bytes, or does not open
  `secrets.sealed`. It never falls back to a plain file. Its error names
  the credential and the file, and never a secret.
- **A plain file moves in once.** On start with `sealed`, maild moves every
  key of a plain `secrets.json` into `secrets.sealed`, reads each one back,
  and removes the plain file, as it does for the Secret Service and DPAPI.
  The move to the server (`scripts/move-mail-to-lodge.sh`) can therefore
  place a plain `secrets.json` before or after this decision is built.
- **The desktop does not change.** A desktop keeps the Secret Service, and
  Windows keeps DPAPI. `FROSTMAIL_SECRETS=file` stays for CI and
  development, and for a server without systemd credentials, where maild
  warns at start.
- **The key is not backed up with the data.** A backup of
  `/var/lib/frostmail` holds `secrets.sealed` but not the key, which lives
  in `/etc/credstore.encrypted/` and is bound to the host. A restore onto
  another host, or a new TPM2, loses the secrets: maild cannot open
  `secrets.sealed` and refuses to start, naming the file. The owner moves
  the file aside, maild starts with no secrets, and every account shows
  "Sign in again".

## Consequences

- A copy of the state directory that leaves the host (a backup, a
  snapshot, a disk image read elsewhere) no longer holds the owner's
  passwords and refresh tokens in plain text. With a TPM2, a copy of the
  whole disk does not either.
- Nothing changes for root or for `frostmail` on the running host: they
  can still read every secret, as ADR-0041 records.
- The install gains one step that only root can do on the host: make the
  key credential. lodge's host is a VM, and with no virtual TPM the key is
  bound to the host key alone, which a copy of the whole disk carries. A
  virtual TPM for the VM is the owner's choice.
- Rebuilding the host, or moving maild to another host, needs one more
  step: re-seal the key there, or accept a Sign In Again for each account.
  The move script's export from the desktop carries plain secrets, and the
  new host seals them on its first start.
- maild gains a sealed file store (about the size of the DPAPI one), a
  test with a key in a temporary credentials directory, and the refusal
  above. `packaging/lodge/frostmail.service`,
  [specs/lodge-app.md](../specs/lodge-app.md) and
  [design/accounts.md](../design/accounts.md#secrets) change with it, and
  the move script accepts `FROSTMAIL_SECRETS=sealed`.
- The secrets exist in plain text twice during the move: in the export on
  the desktop and its copy on the host. Both are 0600 in 0700 directories,
  the copy travels over SSH on the tailnet, and the script tells the owner
  to remove both once the server's maild runs.

## Alternatives considered

- **Keep the plain 0600 file:** the least work, and it guards the running
  host as well as any store here can. But every backup and snapshot of
  `/var/lib` would carry the owner's refresh tokens in plain text, for
  years, wherever the backups go. The sealed file costs little more.
- **The secrets themselves as systemd credentials:** a credential is
  read-only to the service and read once at its start. maild could not
  store a new sign-in or Microsoft's rotated refresh tokens, so every
  token refresh would need root.
- **A Secret Service on the server** (a keyring daemon in a session for
  `frostmail`): it needs a session bus and an unlock at each boot. An
  unattended unlock needs the keyring's password on disk, which is the
  plain file again, behind one more daemon.
- **An external secret manager** (Vault, a password manager's service):
  one more service on the network, and its own credential must live on
  the host, which moves the problem and adds a way to fail.
- **lodge keeps mail's secrets:** ADR-0041 keeps sign-in and secrets
  mail's own. lodge's state would then hold the mail credentials of every
  account, in the process that faces the network.
- **Keep the secrets on the desktop, and have the server ask for them:**
  the server's maild must sync and send while the desktop is off.

## References

- Shapes: [plan 0011](../plans/0011-m8-mail-in-lodge.md#phase-7--the-users-mail-moves-to-lodges-host), [design/accounts.md](../design/accounts.md#secrets), [specs/lodge-app.md](../specs/lodge-app.md), `packaging/lodge/frostmail.service`, `scripts/move-mail-to-lodge.sh`
- Builds on: [ADR-0011](0011-sign-in-and-credentials.md), [ADR-0038](0038-windows-as-a-second-platform.md), [ADR-0039](0039-microsoft-accounts-over-imap-smtp-and-graph.md), [ADR-0041](0041-maild-runs-as-a-lodge-app.md)
- systemd: [systemd-creds(1)](https://www.freedesktop.org/software/systemd/man/latest/systemd-creds.html), [systemd.exec(5), Credentials](https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html#Credentials)
