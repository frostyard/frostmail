# 0047 — The .deb makes the secrets key at first start

- **Status:** Accepted (Brian, 2026-10-27: core plan 0010, Phase 3 brief, point (b))
- **Date:** 2026-10-28

## Context

[ADR-0044](0044-seal-the-servers-secrets-with-a-systemd-credential.md)
seals maild's secrets under a key that is a systemd credential,
`/etc/credstore.encrypted/frostmail-secrets-key`. It decided that "the install
seals once", and it accepted that "the install gains one step that only root
can do".

Since then, core
[plan 0010](https://github.com/frostyard/core/blob/main/docs/plans/0010-suite-packaging.md)
(Phase 3) puts maild in Snosi's sysext, and core
[ADR-0058](https://github.com/frostyard/core/blob/main/docs/adr/0058-ship-suite-desktop-apps-as-flatpaks-and-services-as-sysexts.md)
has the sysext `Upholds=` `frostmail.service`. Facts:

- A sysext ships `/usr` only and runs no maintainer script. `deb-check`
  refuses maintainer scripts. So nothing can make the key at install.
- Without the key, `frostmail.service` fails at start (the credential is
  missing), and `Restart=on-failure` with `RestartSec=2` starts it again.
  That is about 80 attempts in 3 minutes. It never reaches the start limit.
- An upheld unit that fails its condition is retried forever
  ([snosi#1100](https://github.com/frostyard/snosi/issues/1100)), so a condition on `frostmail.service` does not help.
- A by-hand step on a Snosi host is a step the host's owner can forget.

## Decision

The .deb ships `frostmail-secrets-key.service`, a oneshot:

- `ConditionPathExists=!/etc/credstore.encrypted/frostmail-secrets-key`: the
  unit skips when the key exists.
- `Type=oneshot` and `RemainAfterExit=yes`; it runs
  `/usr/libexec/frostmail/make-secrets-key` as root.
- The helper makes 32 random bytes and seals them with
  `systemd-creds encrypt --with-key=auto --name=frostmail-secrets-key`, as
  `scripts/seal-mail-secrets.sh --apply` does. It writes the sealed key in a
  private temporary directory beside the key, then publishes it with a hard
  link (`ln`), which fails when the key exists. So it never replaces a key, and a partial file never
  passes for the key. It creates the directory with mode 0700 when it is
  missing, and does not change a directory that exists.
- The unit has light hardening that keeps `/etc` writable and the TPM device
  visible (`ProtectSystem=yes`, no `PrivateDevices=`).
- `frostmail.service` has `Requires=` and `After=` on it.
- Nothing upholds or enables it: it has no `[Install]` section, so
  `frostmail.service` only pulls it, and its failed condition is a clean skip.

`deb-check` and `internal/lodge/package_test.go` check these facts.

## Consequences

- A Snosi host needs no by-hand step. The key is made at the first start of
  `frostmail.service`.
- `scripts/seal-mail-secrets.sh` and the by-hand command stay valid ways to
  make the key earlier. The condition skips the oneshot when the key exists,
  and `ln` never replaces a key that appears meanwhile.
- A restored `secrets.sealed` under another host's key, with no key file
  here, gets a fresh key that cannot open it. maild refuses and names the
  file, as ADR-0044 records. The owner puts the old key back over the new
  one, or moves `secrets.sealed` aside.
- If `systemd-creds` fails, `frostmail.service` fails on the dependency. The
  oneshot's default start limit applies.
- With the rollback drop-in (`FROSTMAIL_SECRETS=file`) the key is still made
  but not used. This is harmless.
- A hand-installed host whose `frostmail.service` is in `/etc/systemd/system`
  keeps its own unit and needs nothing. A host that copies the staged `usr/`
  gets the oneshot and its helper with it.
- The package carries one more unit and one helper script.

## Alternatives considered

- **A maintainer script:** a sysext runs none, and `deb-check` refuses them.
- **maild makes its own key:** a credential is read-only to the service, and
  maild runs as `frostmail`, which cannot write `/etc`.
- **Uphold the oneshot:** a failed condition is retried forever.
- **`ConditionPathExists=` on `frostmail.service`:** the unit is upheld, so a
  failed condition is retried forever.
- **Also skip the oneshot when `secrets.sealed` exists:** `frostmail.service`
  would then fail on the missing credential in a loop, which is no better.
  The decision names the key path only.

## References

- Amends: [ADR-0044](0044-seal-the-servers-secrets-with-a-systemd-credential.md)
  (the install seals once; the extra install step)
- Builds on: [ADR-0041](0041-maild-runs-as-a-lodge-app.md)
- Shapes: [specs/lodge-app.md](../specs/lodge-app.md),
  [design/accounts.md](../design/accounts.md#secrets)
- Core: [plan 0010](https://github.com/frostyard/core/blob/main/docs/plans/0010-suite-packaging.md) (Phase 3 brief, point (b)),
  [ADR-0058](https://github.com/frostyard/core/blob/main/docs/adr/0058-ship-suite-desktop-apps-as-flatpaks-and-services-as-sysexts.md),
  [ADR-0030](https://github.com/frostyard/core/blob/main/docs/adr/0030-no-requiredby-enablement-in-shipped-units.md)
  (no `RequiredBy=` in a shipped unit)
