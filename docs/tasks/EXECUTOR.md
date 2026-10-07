# Executor rules

You are implementing one task card. The card is the whole job.

1. Read the card, then every file under "Read first", before writing code.
2. Change only the files listed under `touch` in the card's front matter.
   Create them if they do not exist. Never create, edit, rename or delete any
   other file.
3. Never edit the given test files. They define the contract. If a test seems
   wrong, stop and explain why in your final message instead of changing it.
4. Never edit `go.mod`, `go.sum`, `package.json`, `pnpm-lock.yaml`,
   anything under `schema/`, `api/zz_generated.go`, `app/src/rpc/gen/`, or a
   migration under `internal/store/migrations/`. Use only the dependencies
   already in the module.
5. Run commands through make: `make accept T=NNNN` runs the card's tests,
   `make check` runs formatting, vet, lint and every unit test. Both must pass.
6. Fix the code, not the check: no `//nolint`, no skipped tests, no deleted
   assertions.
7. After five failed attempts at the same failure, stop and write what you
   tried and what you think is wrong in your final message.

## Go conventions

- `context.Context` is the first parameter of anything that does I/O.
- Wrap errors with context: `fmt.Errorf("insert account: %w", err)`. Error
  strings are lowercase with no trailing punctuation.
- No mutable package-level variables. Compiled regexps and constant tables at
  package level are fine.
- Every exported identifier has a doc comment that starts with its name.
- Prefer the standard library and the helpers the card names over new code.
- Keep functions under 60 lines.

## TypeScript conventions

- Strict mode, no `any`, no non-null assertions (`!`).
- Call maild only through the generated client in `app/src/rpc/gen/api.ts`.
- Never use `dangerouslySetInnerHTML`.
