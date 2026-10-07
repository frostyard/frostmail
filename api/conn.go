package api

import "context"

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
	c, _ := ctx.Value(connKey{}).(Conn)
	return c
}
