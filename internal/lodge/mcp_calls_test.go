package lodge_test

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/lodge"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// countingMessages counts the message reads maild answers, so a tool that
// reads many messages shows how many calls it makes (issue #292: 1 call
// per message became 1 call for the batch).
type countingMessages struct {
	api.MessageService
	get, getMany, body atomic.Int64
}

func (m *countingMessages) Get(ctx context.Context, p *api.MessageGetParams) (*api.Message, error) {
	m.get.Add(1)
	return m.MessageService.Get(ctx, p)
}

func (m *countingMessages) GetMany(ctx context.Context, p *api.MessageGetManyParams) ([]api.Message, error) {
	m.getMany.Add(1)
	return m.MessageService.GetMany(ctx, p)
}

func (m *countingMessages) Body(ctx context.Context, p *api.MessageBodyParams) (*api.Body, error) {
	m.body.Add(1)
	return m.MessageService.Body(ctx, p)
}

// reset zeroes the counters, so a test counts the calls of one tool alone.
func (m *countingMessages) reset() {
	m.get.Store(0)
	m.getMany.Store(0)
	m.body.Store(0)
}

// TestTheReadToolsReadMessagesInBatches: search_messages and read_thread
// read the headers of their messages in 1 message.getMany call and never
// in 1 message.get call per message, and their answers keep every link
// and text they had; a message's text stays 1 message.body call (issue
// #292).
func TestTheReadToolsReadMessagesInBatches(t *testing.T) {
	var c countingMessages
	srv, acct, thread, _ := seedThreadWith(t, rpctest.Options{Services: func(s *api.Services) {
		c.MessageService = s.Message
		s.Message = &c
	}})
	l := startLodge(t, lodge.NewServer(lodge.Options{Ctx: t.Context(), Version: "test",
		RPC: srv.RPC, Router: srv.Router, Log: discard()}))
	cs, err := mcpClient(t, l, "claude", "account="+itoa(acct))
	if err != nil {
		t.Fatal(err)
	}
	c.reset()

	var found struct {
		Total    int64 `json:"total"`
		Messages []struct {
			ID              int64  `json:"id"`
			MessageIDHeader string `json:"message_id_header"`
			URL             string `json:"url"`
		} `json:"messages"`
	}
	if call(t, cs, "search_messages", map[string]any{"limit": 50}, &found) || found.Total != 26 || len(found.Messages) != 26 {
		t.Fatalf("search_messages = %d of %d hits, want 26 of 26", len(found.Messages), found.Total)
	}
	if get, many := c.get.Load(), c.getMany.Load(); get != 0 || many != 1 {
		t.Errorf("search_messages made %d message.get and %d message.getMany calls, want 0 and 1", get, many)
	}
	for _, h := range found.Messages {
		if h.MessageIDHeader == "" || h.URL != "/mail/message/"+strings.ReplaceAll(h.MessageIDHeader, "@", "%40") {
			t.Errorf("search_messages hit %d = %q %q, want its Message-ID header and its URL", h.ID, h.MessageIDHeader, h.URL)
		}
	}

	c.reset()
	var read struct {
		Count    int `json:"count"`
		Messages []struct {
			ID              int64    `json:"id"`
			MessageIDHeader string   `json:"message_id_header"`
			To              []string `json:"to"`
			Text            string   `json:"text"`
		} `json:"messages"`
	}
	if call(t, cs, "read_thread", map[string]any{"message_id": thread[0]}, &read) || read.Count != 25 || len(read.Messages) != 20 {
		t.Fatalf("read_thread = %d of %d messages, want the newest 20 of 25", len(read.Messages), read.Count)
	}
	if get, many, body := c.get.Load(), c.getMany.Load(), c.body.Load(); get != 0 || many != 1 || body != 20 {
		t.Errorf("read_thread made %d message.get, %d message.getMany and %d message.body calls, want 0, 1 and 20",
			get, many, body)
	}
	for i, m := range read.Messages {
		day := 5 + i
		want := fmt.Sprintf("plan-%d@a.test", day)
		if m.ID != thread[day] || m.MessageIDHeader != want || len(m.To) != 1 || m.To[0] != "me@a.test" ||
			!strings.HasPrefix(m.Text, fmt.Sprintf("Plan message %d.", day)) {
			t.Errorf("read_thread message %d = %+v, want %s to me@a.test with its text", i, m, want)
		}
	}
}
