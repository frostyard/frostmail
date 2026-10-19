package api

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Scope is what an agent's MCP token lets it reach (docs/specs/lodge-app.md,
// /mcp): every account, or the accounts named. Lodge passes it in
// Lodge-Scope without reading it; maild's engine enforces it on the agent's
// connection. Read scopes only, for now: a scope that asks to write or send
// does not parse.
type Scope struct {
	// All is account=*: every account, those added later included.
	All bool
	// Accounts are the account IDs of account=<id>[,<id>…], ascending.
	Accounts []int64
}

// ErrScope means a scope does not parse.
var ErrScope = errors.New("scope")

// ParseScope parses account=* or account=<id>[,<id>…], IDs being positive
// decimal integers without leading zeros.
func ParseScope(s string) (Scope, error) {
	v, ok := strings.CutPrefix(s, "account=")
	if !ok {
		return Scope{}, fmt.Errorf("%w %q: want account=* or account=<id>[,<id>…]", ErrScope, s)
	}
	if strings.Contains(v, ";") {
		return Scope{}, fmt.Errorf("%w %q: this maild grants read scopes only", ErrScope, s)
	}
	if v == "*" {
		return Scope{All: true}, nil
	}
	var sc Scope
	for f := range strings.SplitSeq(v, ",") {
		id, err := strconv.ParseInt(f, 10, 64)
		if err != nil || id <= 0 || strconv.FormatInt(id, 10) != f {
			return Scope{}, fmt.Errorf("%w %q: %q is not an account ID", ErrScope, s, f)
		}
		if !slices.Contains(sc.Accounts, id) {
			sc.Accounts = append(sc.Accounts, id)
		}
	}
	slices.Sort(sc.Accounts)
	return sc, nil
}

// String is the scope in ParseScope's grammar.
func (s Scope) String() string {
	if s.All {
		return "account=*"
	}
	ids := make([]string, len(s.Accounts))
	for i, id := range s.Accounts {
		ids[i] = strconv.FormatInt(id, 10)
	}
	return "account=" + strings.Join(ids, ",")
}

// Admits reports whether the scope reaches account id.
func (s Scope) Admits(id int64) bool { return s.All || slices.Contains(s.Accounts, id) }
