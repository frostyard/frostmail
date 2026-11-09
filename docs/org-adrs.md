# Organization ADRs binding frostmail

Decisions from [frostyard/core](https://github.com/frostyard/core/tree/main/docs/adr) this repository follows:

- ADR-0071: the repository baseline (core's `docs/specs/repository-baseline.md`, rows B1 to B16); `internal/workflowcheck` checks the files of rows B1, B2, B6 and B13.
- ADR-0002, ADR-0018, ADR-0029: `AGENTS.md` is canonical; `CLAUDE.md`, `GEMINI.md`, `.cursorrules` and `.github/copilot-instructions.md` are relative symlinks to it, and `.claude/skills` is a symlink to `.agents/skills/`. `.memory/corrections.jsonl` is the corrections inbox.
- ADR-0012, ADR-0053: svu versions with `.svu.yml`; `make bump` tags a tested candidate (ADR-0048 of this repository).
- ADR-0016: reverse-DNS identifiers; the app ID is `org.frostyard.Frostmail`.
- ADR-0019, ADR-0068: risk tiers in the PR template; `.github/risk-paths.json` and the PR triage caller `.github/workflows/risk-paths.yml` label a PR that touches a high-risk path `risk:high`, and label every PR with 1 level from `level:L1` to `level:L4` (ADR-0078).
- ADR-0021: actions pinned to full commit SHAs, `permissions: {}` and `persist-credentials: false` (`internal/workflowcheck`).
- ADR-0023: downloads are version-pinned and checksum-verified (`dev/nsl/provision.sh`).
- ADR-0025: one `docs/` tree in the four-category shape.
- ADR-0043, ADR-0044: tools pinned in `mise.toml` with `mise.lock`; the `make verify` / `check` / `ci` gate triad.
- ADR-0058: the desktop app ships as a Flatpak, maild for lodge as a package that stages under `usr/`.
- ADR-0070: a tool documentation site in `site/` on GitHub Pages (not built yet).
- ADR-0076: every default branch requires status checks.
- ADR-0077: `ready-for-human` is bound to the head commit; the triage workflow removes it when the head moves after the label, or when CI is not green at the label.
- ADR-0078: pull requests sort by levels `L1` to `L4`; `.github/risk-paths.json` is schema version 2 (amends ADR-0068, whose `risk:high` stays as the alias of L1 and L2).
