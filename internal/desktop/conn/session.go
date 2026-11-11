package conn

import (
	"context"
	"sync"
	"time"

	"github.com/frostyard/frostmail/api"
)

// State is a session's connection, as the status bar shows it.
type State struct {
	Target    Target
	Connected bool
	Hello     *api.Hello // while connected
	// Err is why the last connection ended or the last dial failed; nil
	// while connected and before the first attempt.
	Err error
	// Attempts counts the dials that failed since the last connection.
	Attempts int
}

// Options configure a Session.
type Options struct {
	// Client names the app in rpc.hello, as "frostmail v0.5.0".
	Client string
	// Retry is the wait between attempts [1s]; Ping the interval of
	// rpc.ping on a lodge connection [30s]; DialTimeout bounds one dial
	// [15s]; ConnectTimeout bounds OnConnect [60s].
	Retry, Ping, DialTimeout, ConnectTimeout time.Duration
	// OnConnect runs with each new client before the session reports it
	// connected: the app subscribes and loads its stores there. Its ctx
	// ends with ConnectTimeout, or when it returns: what lives as long as
	// the connection needs a context of its own. An error closes the
	// client and counts as a failed attempt.
	OnConnect func(ctx context.Context, c *api.Client, h *api.Hello) error
	// OnChange learns every change of State, on the session's goroutine.
	OnChange func(State)
}

// Session keeps one connection to maild for every window of the app: it
// dials, reconnects every Retry after the connection ends, pings lodge,
// and moves to a new target when the user changes the connection.
type Session struct {
	opts     Options
	retarget chan Target
	again    chan struct{}

	mu     sync.Mutex
	target Target
	client *api.Client
	state  State
}

// NewSession returns a session for t; Run starts it.
func NewSession(t Target, o Options) *Session {
	if o.Retry == 0 {
		o.Retry = time.Second
	}
	if o.Ping == 0 {
		o.Ping = 30 * time.Second
	}
	if o.DialTimeout == 0 {
		o.DialTimeout = 15 * time.Second
	}
	if o.ConnectTimeout == 0 {
		o.ConnectTimeout = 60 * time.Second
	}
	return &Session{opts: o, target: t, state: State{Target: t}, retarget: make(chan Target, 1), again: make(chan struct{}, 1)}
}

// Client is the connected client, nil while there is none. A call on a
// client that has just closed fails; the caller tries again after the
// next OnConnect.
func (s *Session) Client() *api.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client
}

// State is the connection's state now.
func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Target is the maild the session connects to. During OnConnect it is the
// one that client reached: only the session's goroutine changes it.
func (s *Session) Target() Target { return s.currentTarget() }

// Same reports whether t and u name one maild: IDs from one mean the same
// on the other.
func (t Target) Same(u Target) bool {
	if t.Remote() || u.Remote() {
		return t.Remote() && u.Remote() && t.Lodge.Origin() == u.Lodge.Origin()
	}
	return t.Socket == u.Socket
}

// SetTarget ends the current connection and connects to t instead (the
// Server pane's change of connection).
func (s *Session) SetTarget(t Target) {
	select {
	case <-s.retarget: // a target not yet taken is replaced
	default:
	}
	s.retarget <- t
}

// Retry ends the wait before the next attempt, for a Retry button.
func (s *Session) Retry() {
	select {
	case s.again <- struct{}{}:
	default:
	}
}

// Run connects and keeps the connection until ctx ends.
func (s *Session) Run(ctx context.Context) {
	attempts := 0
	for ctx.Err() == nil {
		t := s.currentTarget()
		dctx, cancel := context.WithTimeout(ctx, s.opts.DialTimeout)
		c, h, err := Dial(dctx, t, s.opts.Client)
		cancel()
		if err == nil && s.opts.OnConnect != nil {
			cctx, done := context.WithTimeout(ctx, s.opts.ConnectTimeout)
			if err = s.opts.OnConnect(cctx, c, h); err != nil {
				_ = c.Close()
			}
			done()
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			attempts++
			s.set(nil, State{Target: t, Err: err, Attempts: attempts})
			s.wait(ctx)
			continue
		}
		attempts = 0
		s.set(c, State{Target: t, Connected: true, Hello: h})
		err = s.hold(ctx, c, t)
		_ = c.Close()
		if ctx.Err() != nil {
			s.set(nil, State{Target: t})
			return
		}
		s.set(nil, State{Target: s.currentTarget(), Err: err})
	}
}

// hold keeps c until it ends, the target changes or ctx ends, pinging
// lodge meanwhile; it returns why the connection ended (nil for a change
// of target).
func (s *Session) hold(ctx context.Context, c *api.Client, t Target) error {
	var ping <-chan time.Time
	if t.Remote() {
		tick := time.NewTicker(s.opts.Ping)
		defer tick.Stop()
		ping = tick.C
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.Done():
			return c.Err()
		case next := <-s.retarget:
			s.mu.Lock()
			s.target = next
			s.mu.Unlock()
			return nil
		case <-ping:
			go func() {
				pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
				_ = c.RPC().Ping(pctx, &api.RPCPingParams{})
			}()
		}
	}
}

// wait sleeps Retry, or until the target changes or ctx ends.
func (s *Session) wait(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-time.After(s.opts.Retry):
	case <-s.again:
	case next := <-s.retarget:
		s.mu.Lock()
		s.target = next
		s.mu.Unlock()
	}
}

func (s *Session) currentTarget() Target {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.target
}

func (s *Session) set(c *api.Client, st State) {
	s.mu.Lock()
	s.client, s.state = c, st
	s.mu.Unlock()
	if s.opts.OnChange != nil {
		s.opts.OnChange(st)
	}
}
