package mcpserver

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/frostyard/lodge/kit/lodgeapp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frostyard/frostmail/api"
)

// HTTPOptions configure Handler.
type HTTPOptions struct {
	// Open connects a new session to maild as caller agent, bound to the
	// token's name and scope, and returns a client that completed
	// rpc.hello. Handler closes it when the session ends.
	Open func(ctx context.Context, token string, scope api.Scope) (*api.Client, error)
	// Version is reported to the MCP host as the server's.
	Version string
	// Log receives sessions opened; nil discards them.
	Log *slog.Logger
}

// sessionHeader is MCP Streamable HTTP's session header.
const sessionHeader = "Mcp-Session-Id"

// Handler serves MCP's Streamable HTTP transport for lodge's /mcp, for
// caller agent (lodgeapp.CallerFrom). Each initialize gets a server whose
// connection to maild carries the token's scope. Sessions end when the
// client deletes them; binding them to their token, bounding them and
// ending idle ones is lodge/kit's mcpsession, which ends a session with a
// DELETE.
func Handler(o HTTPOptions) http.Handler {
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	h := &httpSessions{o: o, clients: map[string]*api.Client{}}
	h.next = sdk.NewStreamableHTTPHandler(h.server, &sdk.StreamableHTTPOptions{Logger: o.Log})
	return h
}

type httpSessions struct {
	o    HTTPOptions
	next http.Handler

	mu      sync.Mutex
	clients map[string]*api.Client // by MCP session ID
}

// opening carries a new session's maild client from server back to
// ServeHTTP, which keeps it under the session's ID.
type opening struct{ client *api.Client }

type openingKey struct{}

func (h *httpSessions) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get(sessionHeader)
	if id != "" {
		h.next.ServeHTTP(w, r)
		if r.Method == http.MethodDelete {
			h.forget(id)
		}
		return
	}
	op := &opening{}
	rec := &sessionRecorder{ResponseWriter: w}
	h.next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), openingKey{}, op)))
	if op.client == nil {
		return
	}
	if rec.id == "" {
		_ = op.client.Close()
		return
	}
	h.mu.Lock()
	if old := h.clients[rec.id]; old != nil {
		_ = old.Close()
	}
	h.clients[rec.id] = op.client
	h.mu.Unlock()
}

// forget closes an ended session's maild client.
func (h *httpSessions) forget(id string) {
	h.mu.Lock()
	c := h.clients[id]
	delete(h.clients, id)
	h.mu.Unlock()
	if c != nil {
		_ = c.Close()
	}
}

// server builds a new session's server from its request's token and
// scope; nil answers 400. A scope that does not parse never reaches here
// (mcpsession refuses it with 403), but is refused again.
func (h *httpSessions) server(r *http.Request) *sdk.Server {
	op, _ := r.Context().Value(openingKey{}).(*opening)
	c, ok := lodgeapp.CallerFrom(r.Context())
	if op == nil || !ok || c.Kind != lodgeapp.Agent || c.Token == "" {
		return nil
	}
	scope, err := api.ParseScope(c.Scope)
	if err != nil {
		h.o.Log.Warn("an MCP session with a scope that does not parse", "token", c.Token, "err", err)
		return nil
	}
	client, err := h.o.Open(r.Context(), c.Token, scope)
	if err != nil {
		h.o.Log.Error("connect an MCP session to maild", "token", c.Token, "err", err)
		return nil
	}
	op.client = client
	h.o.Log.Info("MCP session", "token", c.Token, "scope", scope.String())
	return newServer(Options{Client: client, Version: h.o.Version})
}

// sessionRecorder notes the session ID a response issues, keeping streams
// streaming.
type sessionRecorder struct {
	http.ResponseWriter
	id    string
	wrote bool
}

func (r *sessionRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.wrote = true
		if code/100 == 2 {
			r.id = r.Header().Get(sessionHeader)
		}
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *sessionRecorder) Write(p []byte) (int, error) {
	if !r.wrote {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(p)
}

// Flush keeps streams streaming.
func (r *sessionRecorder) Flush() {
	if !r.wrote {
		r.WriteHeader(http.StatusOK)
	}
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the connection.
func (r *sessionRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
