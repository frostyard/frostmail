package api

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Scope is what an agent's MCP token lets it reach (docs/specs/lodge-app.md,
// /mcp): every account, or the accounts named, to read, and with ;write and
// ;send to change and to send. Lodge passes it in Lodge-Scope without
// reading it; maild's router admits the methods it covers (schema/rpc's
// scope), and the engine the accounts it names, on the agent's connection.
type Scope struct {
	// All is account=*: every account, those added later included.
	All bool
	// Accounts are the account IDs of account=<id>[,<id>…], ascending.
	Accounts []int64
	// Write is ;write: flags, moves and drafts.
	Write bool
	// Send is ;send: queueing a draft to send.
	Send bool
}

// ScopeNeed is what a method needs of an agent's scope beyond reading
// (schema/rpc's scope).
type ScopeNeed string

// What methods need.
const (
	NeedRead  ScopeNeed = ""
	NeedWrite ScopeNeed = "write"
	NeedSend  ScopeNeed = "send"
)

// Grants reports whether the scope covers what a method needs.
func (s Scope) Grants(n ScopeNeed) bool {
	switch n {
	case NeedWrite:
		return s.Write
	case NeedSend:
		return s.Send
	}
	return true
}

// MethodNeed is what method needs of an agent's scope.
func MethodNeed(method string) ScopeNeed { return methodNeeds[method] }

// ErrScope means a scope does not parse.
var ErrScope = errors.New("scope")

// ParseScope parses account=* or account=<id>[,<id>…], IDs being positive
// decimal integers without leading zeros, then optionally ;write and then
// ;send, each at most once.
func ParseScope(s string) (Scope, error) {
	v, ok := strings.CutPrefix(s, "account=")
	if !ok {
		return Scope{}, fmt.Errorf("%w %q: want account=* or account=<id>[,<id>…], then ;write or ;send", ErrScope, s)
	}
	var sc Scope
	v, mods, _ := strings.Cut(v, ";")
	if mods != "" || strings.HasSuffix(s, ";") {
		for m := range strings.SplitSeq(mods, ";") {
			switch {
			case m == "write" && !sc.Write && !sc.Send:
				sc.Write = true
			case m == "send" && !sc.Send:
				sc.Send = true
			default:
				return Scope{}, fmt.Errorf("%w %q: %q is not ;write or ;send, once each and in that order", ErrScope, s, m)
			}
		}
	}
	if v == "*" {
		sc.All = true
		return sc, nil
	}
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
	out := "account=*"
	if !s.All {
		ids := make([]string, len(s.Accounts))
		for i, id := range s.Accounts {
			ids[i] = strconv.FormatInt(id, 10)
		}
		out = "account=" + strings.Join(ids, ",")
	}
	if s.Write {
		out += ";write"
	}
	if s.Send {
		out += ";send"
	}
	return out
}

// Admits reports whether the scope reaches account id.
func (s Scope) Admits(id int64) bool { return s.All || slices.Contains(s.Accounts, id) }
