# Organization ADRs binding frostmail

Decisions from [frostyard/core](https://github.com/frostyard/core/tree/main/docs/adr) this repository follows:

- ADR-0002, ADR-0029: `AGENTS.md` is canonical; `CLAUDE.md` and `GEMINI.md` are symlinks.
- ADR-0016: reverse-DNS identifiers; the app ID is `org.frostyard.Frostmail`.
- ADR-0023: downloads are version-pinned and checksum-verified (`dev/nsl/provision.sh`).
- ADR-0025: one `docs/` tree in the four-category shape.
- ADR-0043, ADR-0044: tools pinned in `mise.toml` with `mise.lock`; the `make verify` / `check` / `ci` gate triad.
