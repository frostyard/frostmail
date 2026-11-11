package conn_test

import (
	"context"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// A maild that accepts and then stops answering must not hold the session
// in OnConnect for good: a change of target waits behind it.
func TestOnConnectIsBounded(t *testing.T) {
	srv := rpctest.Start(t)
	states := make(chan conn.State, 16)
	s := conn.NewSession(conn.Target{Socket: srv.Socket}, conn.Options{
		Client: "frostmail test", Retry: time.Hour, ConnectTimeout: 20 * time.Millisecond,
		OnConnect: func(ctx context.Context, _ *api.Client, _ *api.Hello) error {
			<-ctx.Done()
			return ctx.Err()
		},
		OnChange: func(st conn.State) { states <- st },
	})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	select {
	case st := <-states:
		if st.Connected || st.Err == nil {
			t.Errorf("state = %+v, want a failed attempt", st)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("OnConnect was never ended")
	}
}

func TestTargetsNameOneMaild(t *testing.T) {
	a, err := conn.ParseLodge("https://a.example.ts.net")
	if err != nil {
		t.Fatal(err)
	}
	a2, err := conn.ParseLodge("https://A.example.ts.net/")
	if err != nil {
		t.Fatal(err)
	}
	b, err := conn.ParseLodge("https://b.example.ts.net")
	if err != nil {
		t.Fatal(err)
	}
	local := conn.Target{Socket: "/run/a.sock"}
	for _, tc := range []struct {
		x, y conn.Target
		want bool
	}{
		{local, conn.Target{Socket: "/run/a.sock"}, true},
		{local, conn.Target{Socket: "/run/b.sock"}, false},
		{local, conn.Target{Socket: "/run/a.sock", Lodge: &a}, false},
		{conn.Target{Lodge: &a}, conn.Target{Lodge: &a2}, true},
		{conn.Target{Lodge: &a}, conn.Target{Lodge: &b}, false},
	} {
		if got := tc.x.Same(tc.y); got != tc.want {
			t.Errorf("%v same as %v = %v", tc.x, tc.y, got)
		}
	}
}
