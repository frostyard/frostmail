package api

import (
	"context"
	"slices"

	"github.com/frostyard/lodge/kit/jsonrpc"
)

// Conn is the client connection a request arrived on, for services that
// keep per-connection state, such as views.
type Conn interface {
	// Notify sends a transient event to this connection only.
	Notify(Event) error
	// Done is closed when the connection ends.
	Done() <-chan struct{}
}

type connKey struct{}

// WithConn returns ctx carrying c; the RPC server sets it for every request.
func WithConn(ctx context.Context, c Conn) context.Context {
	return context.WithValue(ctx, connKey{}, c)
}

// ConnFrom returns the connection a request arrived on, or nil.
func ConnFrom(ctx context.Context) Conn {
	if c, ok := ctx.Value(connKey{}).(Conn); ok {
		return c
	}
	if c := jsonrpc.ConnFrom(ctx); c != nil {
		return rpcConn{c}
	}
	return nil
}

// rpcConn is the server's connection as a Conn.
type rpcConn struct{ *jsonrpc.Conn }

// Notify sends ev as a transient event.
func (c rpcConn) Notify(ev Event) error {
	env, err := NewEnvelope(0, ev)
	if err != nil {
		return err
	}
	return c.Conn.Notify(EventMethod, env)
}

// Caller is who a connection serves (docs/specs/lodge-app.md). Login,
// NodeName, DisplayName and Session come from lodge for a session; Token
// and Scope for an agent, whose scope the engine enforces.
type Caller struct {
	Kind        CallerKind
	Login       string
	NodeName    string
	DisplayName string
	Session     string
	Token       string
	Scope       Scope
}

type callerKey struct{}

// WithCaller returns ctx carrying c; the RPC server sets it for every
// request on a connection that is not local.
func WithCaller(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, c)
}

// CallerFrom returns the caller a request arrived for: a local caller when
// none was set.
func CallerFrom(ctx context.Context) Caller {
	if c, ok := ctx.Value(callerKey{}).(Caller); ok {
		return c
	}
	return Caller{Kind: CallerKindLocal}
}

// Admits reports whether method admits callers of kind, by the schema's
// callers. An unknown method admits none.
func Admits(method string, kind CallerKind) bool {
	return slices.Contains(methodCallers[method], kind)
}
