# 0001 — Record architecture decisions

- **Status:** Accepted
- **Date:** 2026-10-07

## Context

Frostmail is built by a planner model, a local executor model and a human.
Each needs the reasons behind the design without re-deriving them.

## Decision

Significant decisions are recorded as ADRs in `docs/adr/` from core's
`TEMPLATE.md`, numbered in order, and are immutable once accepted.

## Consequences

Decisions are reviewable and linkable from code. Reversing one costs a new
ADR.

## Alternatives considered

- **Decisions in commit messages only:** not discoverable from the code.

## References

- Builds on: frostyard/core ADR-0001, ADR-0025
