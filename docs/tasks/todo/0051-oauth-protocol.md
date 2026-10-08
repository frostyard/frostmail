---
id: "0051"
title: Write the OAuth protocol pieces
milestone: M4
size: M
touch:
  - internal/oauth/protocol.go
given:
  - internal/oauth/protocol_test.go
acceptance: go test ./internal/oauth -count=1
---
# T-0051: Write the OAuth protocol pieces

## Goal

maild signs in to Gmail with OAuth: an authorization-code flow with PKCE,
token responses, and the SASL XOAUTH2 mechanism for IMAP and SMTP
(`docs/design/accounts.md`, Sign-in; ADR-0011). Implement the pure
protocol functions; the planner builds the loopback flow and the token
manager on them.

## Read first

- `internal/oauth/oauth.go`: `Endpoint`, `Google`, `Token`, `TokenError`,
  `ChallengeError`.
- `internal/oauth/protocol.go`: the stubs.
- `docs/design/accounts.md`: "Sign-in".
- The given test `internal/oauth/protocol_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the signatures; replace the stubs' doc comments and the "Task T-0051
…" sentence with what each does. Remove `errNotYet`.

- **`NewVerifier(r)`:** read exactly 32 bytes from `r` (`io.ReadFull`; a
  short read is an error) and return them in unpadded base64url
  (`base64.RawURLEncoding`): 43 characters.
- **`Challenge(verifier)`:** the unpadded base64url SHA-256 of the
  verifier (RFC 7636 S256).
- **`AuthURL(e, clientID, redirectURI, state, challenge, loginHint)`:**
  `e.AuthURL + "?" +` the `url.Values` encoding of `response_type=code`,
  `client_id`, `redirect_uri`, `scope` (the endpoint's scopes joined by a
  space), `state`, `code_challenge`, `code_challenge_method=S256`,
  `access_type=offline`, `prompt=consent`, and `login_hint` only when not
  empty.
- **`ParseToken(body, status, now)`:** decode the JSON fields
  `access_token`, `refresh_token`, `expires_in`, `error`,
  `error_description`. When the body decodes and has a non-empty `error`,
  return `&TokenError{Code, Description}` (whatever the status). Otherwise a
  status other than 200 is an error ("token endpoint answered HTTP N"); a
  body that does not decode is an error; an empty `access_token` is an
  error; a negative `expires_in` is an error. Else return `Token{Access,
  Refresh}` with `Expiry = now + expires_in seconds`, or a zero `Expiry`
  when `expires_in` is 0 or missing. None of these other errors is a
  `TokenError`.
- **`XOAuth2(user, accessToken)`:** `"user=" + user + "\x01auth=Bearer " +
  accessToken + "\x01\x01"` as bytes.
- **`ParseXOAuth2Error(b)`:** the server's failure details, as JSON
  (starting with `{`) or as standard base64 of JSON. Decode `status`,
  `schemes`, `scope` and return `&ChallengeError{...}`. Anything that does
  not decode, or has no `status`, returns a plain error
  ("xoauth2: unreadable server error").

## Tests (given, do not edit)

`internal/oauth/protocol_test.go`: TestPKCE, TestAuthURL, TestParseToken,
TestXOAuth2.

## Gotchas

- The repository's JSON package is `encoding/json/v2`.
- `url.Values.Encode` sorts the keys; that is fine.
- Never use `math/rand`; the caller passes `crypto/rand.Reader`.

## Out of scope

HTTP calls, the loopback listener, token storage and refresh (planner), and
every file except `internal/oauth/protocol.go`.

## Done when

`make accept T=0051` and `make check` pass, and only
`internal/oauth/protocol.go` changed.
