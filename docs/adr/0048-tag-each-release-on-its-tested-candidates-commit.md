# 0048 — Tag each release on its tested candidate's commit

- **Status:** Accepted
- **Date:** 2026-10-29

## Context

A release `vX.Y.Z` ships 2 artifacts. The desktop Flatpak
([ADR-0046](0046-the-desktop-flatpak-carries-its-own-maild.md)) releases by
promotion: a `vX.Y.Z-rc.N` tag builds, publishes and attests a candidate,
the clean-install gate tests it, and the Promote workflow copies that digest
to `vX.Y.Z` and `latest`, so nothing is built again
([plan 0013](../plans/0013-m6-the-standalone-desktop-flatpak.md), Phases 8 to
10). The server's `frostyard-maild_X.Y.Z_amd64.deb` (lodge's `mail` app) is
built by `.github/workflows/release.yml` from the commit of the git tag
`vX.Y.Z`.

`make bump` made that tag on main's head
([core ADR-0012](https://github.com/frostyard/core/blob/main/docs/adr/0012-svu-versioning-and-rolling-dev-prerelease.md)).
Main can move on after the candidate, so the `.deb` of `vX.Y.Z` could hold
commits that the Flatpak of `vX.Y.Z` does not. Brian decided (T492a,
2026-10-29): the `.deb` and the Flatpak of one version hold the same code.

## Decision

The git tag `vX.Y.Z` marks the commit of the candidate that passed the gate,
never a later one.

- `make bump CANDIDATE=vX.Y.Z-rc.N` is still the only way to tag a release.
  It takes the version from the candidate, runs `make verify` on the
  candidate's commit (in a temporary worktree), and tags that commit. It
  refuses a candidate tag that is missing or not on main, a version tag that
  exists, and a version that is not newer than the last release.
- `release.yml` refuses a `vX.Y.Z` tag whose commit is not the commit of a
  `vX.Y.Z-rc.N` tag, before it builds, and checks that its checkout is that
  commit. It builds and publishes no Flatpak.
- The Promote workflow refuses unless the git tags `vX.Y.Z` and the
  candidate name the same commit (the compare API answers `identical`).

## Consequences

- A release's `.deb` and Flatpak are built from one commit, and 2 workflows
  check it, so a mistake in `make bump` cannot break the rule unseen.
- Every release, a fix for the server only too, needs a candidate tag first,
  and the candidate's Flatpak is built and published to GHCR under its `-rc`
  tag. The gate's depth for such a release is Brian's call; the tags are not
  optional.
- `make bump` no longer runs `svu next`: the candidate's tag fixes the
  version. `svu next` names the version when Brian pushes the candidate.
- `make bump` no longer needs a clean, current `main` checkout: it verifies
  and tags the candidate's commit, whatever is checked out.

## Alternatives considered

- **Promote makes the version tag:** a tag that a workflow pushes with
  `GITHUB_TOKEN` starts no other workflow, so `release.yml` would need a
  second trigger, and a workflow would tag releases, which core ADR-0012
  gives to `make bump`.
- **Tag main's head, as before:** the `.deb` could hold later commits than
  the Flatpak, against Brian's decision.

## References

- Shapes: [plan 0013](../plans/0013-m6-the-standalone-desktop-flatpak.md)
  (Phase 10), [design/desktop.md](../design/desktop.md),
  [specs/lodge-app.md](../specs/lodge-app.md) (Release)
- Builds on: [ADR-0046](0046-the-desktop-flatpak-carries-its-own-maild.md),
  [ADR-0045](0045-the-desktop-app-builds-as-a-flatpak-in-ci.md),
  [core ADR-0012](https://github.com/frostyard/core/blob/main/docs/adr/0012-svu-versioning-and-rolling-dev-prerelease.md)
