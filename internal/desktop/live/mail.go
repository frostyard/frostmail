package live

import (
	"context"
	"maps"
	"sync"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
)

// MailState is what the mail store holds: everything the sidebar, the
// toolbar and the settings read, as of one moment. Its slices and maps are
// replaced, never changed in place, so a snapshot can be read without a
// lock.
type MailState struct {
	Accounts  []api.Account
	Mailboxes []api.Mailbox
	Sync      map[int64]api.SyncStatus // by account
	Outbox    []api.OutboxItem
	VIPs      []api.Vip
	Blocked   []api.BlockedSender
	Settings  *api.Settings
	Smart     []api.SmartMailbox
	Rules     []api.Rule
	// Counts and SmartCounts are the sidebar's counts of its built-in
	// sources and its smart mailboxes.
	Counts      *mail.Counts
	SmartCounts map[int64]api.ViewCount
	// FollowUp is the conversations waiting on other people; nil until
	// loaded. A failed refresh keeps the last answer.
	FollowUp *api.Awaiting
	// Loaded is true once the first load answered.
	Loaded bool
}

// The debounces of the reloads that bursts of events cause.
const (
	mailboxDebounce  = 100 * time.Millisecond
	outboxDebounce   = 50 * time.Millisecond
	countsDebounce   = 300 * time.Millisecond
	followUpDebounce = time.Second
	followUpEvery    = 15 * time.Minute
)

// FollowUpLimit and FollowUpDays are the Follow Up section's: at most 25
// shown, of mail with no reply for 3 days or more.
const (
	FollowUpLimit = 25
	FollowUpDays  = 3
)

// Mail is the mail store: loaded when a connection opens, and kept current
// from events (session.tsx's reload and wire).
type Mail struct {
	notify func()

	mu        sync.Mutex
	c         *api.Client
	state     MailState
	mbTimer   *time.Timer
	outTimer  *time.Timer
	cntTimer  *time.Timer
	fuTimer   *time.Timer
	fuTick    *time.Ticker
	off       func()
	ctx       context.Context
	cancelCtx context.CancelFunc
}

// NewMail returns an empty store that calls notify after every change.
func NewMail(notify func()) *Mail { return &Mail{notify: notify} }

// State is the store's state now.
func (m *Mail) State() MailState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// Connect loads everything from c and keeps it current from events until
// the next Connect. The accounts, mailboxes, sync and outbox load
// together and fail together; the preferences load on their own, so a
// maild without one still shows mail.
func (m *Mail) Connect(ctx context.Context, c *api.Client, events *Events) error {
	// Listen first, so a change made during the load reloads after it.
	m.mu.Lock()
	m.stopLocked()
	m.c = c
	m.ctx, m.cancelCtx = context.WithCancel(context.Background())
	m.off = events.On(m.event)
	m.mu.Unlock()

	accounts, err := c.Account().List(ctx, &api.AccountListParams{})
	if err != nil {
		return err
	}
	mailboxes, err := c.Mailbox().List(ctx, &api.MailboxListParams{})
	if err != nil {
		return err
	}
	syncs, err := c.Sync().Status(ctx, &api.SyncStatusParams{})
	if err != nil {
		return err
	}
	outbox, err := c.Outbox().List(ctx, &api.OutboxListParams{})
	if err != nil {
		return err
	}
	byAccount := make(map[int64]api.SyncStatus, len(syncs))
	for _, s := range syncs {
		byAccount[s.AccountID] = s
	}
	m.update(c, func(s *MailState) {
		s.Accounts, s.Mailboxes, s.Sync, s.Outbox, s.Loaded = accounts, mailboxes, byAccount, outbox, true
	})
	for _, load := range []func(){m.loadVIPs, m.loadBlocked, m.loadSettings, m.loadSmart, m.loadRules, m.loadFollowUp} {
		go load()
	}
	m.startFollowUpTick()
	return nil
}

func (m *Mail) startFollowUpTick() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.c == nil {
		return
	}
	tick := time.NewTicker(followUpEvery)
	m.fuTick = tick
	ctx := m.ctx
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				m.loadFollowUp()
			}
		}
	}()
}

// Disconnect stops listening to the connection that ended; the state stays
// as it was until the next Connect.
func (m *Mail) Disconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
}

// Forget empties the store, when the app moves to another maild: nothing
// one maild said holds on another, and a load that fails there must not
// leave it standing. A reconnect to the same maild keeps what it has.
func (m *Mail) Forget() {
	m.mu.Lock()
	m.state = MailState{}
	m.mu.Unlock()
	m.notify()
}

func (m *Mail) stopLocked() {
	if m.off != nil {
		m.off()
		m.off = nil
	}
	if m.cancelCtx != nil {
		m.cancelCtx()
	}
	for _, t := range []*time.Timer{m.mbTimer, m.outTimer, m.cntTimer, m.fuTimer} {
		if t != nil {
			t.Stop()
		}
	}
	if m.fuTick != nil {
		m.fuTick.Stop()
	}
	m.mbTimer, m.outTimer, m.cntTimer, m.fuTimer, m.fuTick, m.c = nil, nil, nil, nil, nil, nil
}

func (m *Mail) event(ev api.Event) {
	switch ev.(type) {
	case api.MailboxChanged, api.VipChanged, api.SettingsChanged, api.SmartChanged:
		m.debounce(&m.cntTimer, countsDebounce, m.loadCounts)
	}
	switch ev.(type) {
	case api.MailboxChanged, api.MessageChanged, api.MessageRemoved, api.VipChanged, api.AccountChanged, api.SettingsChanged:
		m.debounce(&m.fuTimer, followUpDebounce, m.loadFollowUp)
	}
	switch e := ev.(type) {
	case api.MailboxChanged, api.AccountChanged:
		m.debounce(&m.mbTimer, mailboxDebounce, m.loadAccountsAndMailboxes)
	case api.OutboxChanged:
		m.debounce(&m.outTimer, outboxDebounce, m.loadOutbox)
	case api.VipChanged:
		go m.loadVIPs()
	case api.BlockChanged:
		go m.loadBlocked()
	case api.SettingsChanged:
		go m.loadSettings()
	case api.SmartChanged:
		go m.loadSmart()
	case api.RuleChanged:
		go m.loadRules()
	case api.SyncProgress:
		c, _ := m.client()
		m.update(c, func(s *MailState) {
			next := make(map[int64]api.SyncStatus, len(s.Sync)+1)
			maps.Copy(next, s.Sync)
			next[e.Status.AccountID] = e.Status
			s.Sync = next
		})
	}
}

func (m *Mail) debounce(t **time.Timer, d time.Duration, fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if *t != nil {
		(*t).Stop()
	}
	*t = time.AfterFunc(d, fn)
}

// client is the store's client and its context, nil once disconnected.
func (m *Mail) client() (*api.Client, context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.c, m.ctx
}

// update changes the state unless the store has left c, and notifies: an
// answer that comes after the next Connect is another connection's.
func (m *Mail) update(c *api.Client, fn func(*MailState)) {
	m.mu.Lock()
	if m.c == nil || m.c != c {
		m.mu.Unlock()
		return
	}
	fn(&m.state)
	m.mu.Unlock()
	m.notify()
}

// loadInto runs one load on the store's client and stores its result.
func loadInto[T any](m *Mail, call func(context.Context, *api.Client) (T, error), store func(*MailState, T)) {
	c, ctx := m.client()
	loadOn(ctx, m, c, call, store)
}

// loadOn is loadInto on a client the caller read with what it asks for.
func loadOn[T any](ctx context.Context, m *Mail, c *api.Client, call func(context.Context, *api.Client) (T, error), store func(*MailState, T)) {
	if c == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	v, err := call(ctx, c)
	if err != nil {
		return
	}
	m.update(c, func(s *MailState) { store(s, v) })
}

func (m *Mail) loadAccountsAndMailboxes() {
	type both struct {
		a []api.Account
		b []api.Mailbox
	}
	loadInto(m, func(ctx context.Context, c *api.Client) (both, error) {
		a, err := c.Account().List(ctx, &api.AccountListParams{})
		if err != nil {
			return both{}, err
		}
		b, err := c.Mailbox().List(ctx, &api.MailboxListParams{})
		return both{a, b}, err
	}, func(s *MailState, v both) { s.Accounts, s.Mailboxes = v.a, v.b })
}

func (m *Mail) loadOutbox() {
	loadInto(m, func(ctx context.Context, c *api.Client) ([]api.OutboxItem, error) {
		return c.Outbox().List(ctx, &api.OutboxListParams{})
	}, func(s *MailState, v []api.OutboxItem) { s.Outbox = v })
}

func (m *Mail) loadVIPs() {
	loadInto(m, func(ctx context.Context, c *api.Client) ([]api.Vip, error) {
		return c.Vip().List(ctx, &api.VipListParams{})
	}, func(s *MailState, v []api.Vip) { s.VIPs = v })
	m.loadCounts()
}

func (m *Mail) loadFollowUp() {
	limit, days := int64(FollowUpLimit), int64(FollowUpDays)
	loadInto(m, func(ctx context.Context, c *api.Client) (*api.Awaiting, error) {
		return c.Thread().AwaitingOthers(ctx, &api.ThreadAwaitingOthersParams{Limit: &limit, WaitingDays: &days})
	}, func(s *MailState, v *api.Awaiting) { s.FollowUp = v })
}

// loadCounts asks for the counts of VIPs, each VIP group, Flagged, the
// seven flag colors and the smart mailboxes, in one call each.
func (m *Mail) loadCounts() {
	// One read: the queries and their answers belong to one maild.
	m.mu.Lock()
	st, c, conn := m.state, m.c, m.ctx
	m.mu.Unlock()
	groups := mail.VIPGroups(st.VIPs)
	queries := []api.ViewQuery{mail.SourceQuery(mail.Source{Kind: mail.VIPs}, false)}
	for _, g := range groups {
		queries = append(queries, mail.SourceQuery(mail.Source{Kind: mail.VIP, VIPKey: g.Key, Addresses: g.Addresses}, false))
	}
	queries = append(queries, mail.SourceQuery(mail.Source{Kind: mail.Flagged}, false))
	for color := 1; color <= 7; color++ {
		queries = append(queries, mail.SourceQuery(mail.Source{Kind: mail.FlagColor, Color: color}, false))
	}
	loadOn(conn, m, c, func(ctx context.Context, c *api.Client) ([]api.ViewCount, error) {
		return c.View().Count(ctx, &api.ViewCountParams{Queries: queries})
	}, func(s *MailState, r []api.ViewCount) {
		at := func(i int) api.ViewCount {
			if i < len(r) {
				return r[i]
			}
			return api.ViewCount{}
		}
		counts := &mail.Counts{VIPs: at(0), VIP: map[string]api.ViewCount{}, Flagged: at(len(groups) + 1)}
		for i, g := range groups {
			counts.VIP[g.Key] = at(i + 1)
		}
		for i := range counts.Colors {
			counts.Colors[i] = at(len(groups) + 2 + i)
		}
		s.Counts = counts
	})
	if len(st.Smart) == 0 {
		return
	}
	smart := make([]api.ViewQuery, 0, len(st.Smart))
	for _, sm := range st.Smart {
		smart = append(smart, mail.SourceQuery(mail.Source{Kind: mail.Smart, ID: sm.ID}, false))
	}
	loadOn(conn, m, c, func(ctx context.Context, c *api.Client) ([]api.ViewCount, error) {
		return c.View().Count(ctx, &api.ViewCountParams{Queries: smart})
	}, func(s *MailState, r []api.ViewCount) {
		out := make(map[int64]api.ViewCount, len(r))
		for i, sm := range st.Smart {
			if i < len(r) {
				out[sm.ID] = r[i]
			}
		}
		s.SmartCounts = out
	})
}

func (m *Mail) loadBlocked() {
	loadInto(m, func(ctx context.Context, c *api.Client) ([]api.BlockedSender, error) {
		return c.Block().List(ctx, &api.BlockListParams{})
	}, func(s *MailState, v []api.BlockedSender) { s.Blocked = v })
}

func (m *Mail) loadSettings() {
	loadInto(m, func(ctx context.Context, c *api.Client) (*api.Settings, error) {
		return c.Settings().Get(ctx, &api.SettingsGetParams{})
	}, func(s *MailState, v *api.Settings) { s.Settings = v })
}

func (m *Mail) loadSmart() {
	loadInto(m, func(ctx context.Context, c *api.Client) ([]api.SmartMailbox, error) {
		return c.Smart().List(ctx, &api.SmartListParams{})
	}, func(s *MailState, v []api.SmartMailbox) { s.Smart = v })
	m.loadCounts()
}

func (m *Mail) loadRules() {
	loadInto(m, func(ctx context.Context, c *api.Client) ([]api.Rule, error) {
		return c.Rule().List(ctx, &api.RuleListParams{})
	}, func(s *MailState, v []api.Rule) { s.Rules = v })
}
