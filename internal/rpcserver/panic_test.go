package rpcserver_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// panicky is a VIP service whose list panics; its other methods are never
// called.
type panicky struct{ api.VipService }

func (panicky) List(context.Context, *api.VipListParams) ([]api.Vip, error) {
	panic("a bug in a handler")
}

// A handler that panics answers internal; maild and the connection live on.
func TestPanickingHandlerAnswersInternal(t *testing.T) {
	srv := rpctest.StartWith(t, rpctest.Options{Services: func(s *api.Services) { s.Vip = panicky{} }})
	c := srv.Dial(t)
	if _, err := c.Vip().List(t.Context(), &api.VipListParams{}); !errors.Is(err, api.ErrInternal) {
		t.Fatalf("List = %v, want internal", err)
	}
	if _, err := c.Settings().Get(t.Context(), &api.SettingsGetParams{}); err != nil {
		t.Fatalf("the connection after a panic: %v", err)
	}
	if _, err := srv.Dial(t).Settings().Get(t.Context(), &api.SettingsGetParams{}); err != nil {
		t.Fatalf("a new connection after a panic: %v", err)
	}
}

// huge is a VIP service whose list answers more than a message holds; its
// other methods are never called.
type huge struct{ api.VipService }

func (huge) List(context.Context, *api.VipListParams) ([]api.Vip, error) {
	return []api.Vip{{Address: "big@mailtest.test", Name: strings.Repeat("x", api.MaxMessageSize)}}, nil
}

// A reply over the message limit answers internal instead, which the client
// can read; the connection lives on.
func TestAReplyTooLargeAnswersInternal(t *testing.T) {
	srv := rpctest.StartWith(t, rpctest.Options{Services: func(s *api.Services) { s.Vip = huge{} }})
	c := srv.Dial(t)
	if _, err := c.Vip().List(t.Context(), &api.VipListParams{}); !errors.Is(err, api.ErrInternal) {
		t.Fatalf("List = %v, want internal", err)
	}
	if _, err := c.Settings().Get(t.Context(), &api.SettingsGetParams{}); err != nil {
		t.Fatalf("the connection after a reply too large: %v", err)
	}
}
