package engine

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/search"
	"github.com/frostyard/frostmail/internal/store"
)

// The M5 domains (docs/design/organize.md, plans/0008).

// Settings implements the settings domain.
func (e *Engine) Settings() api.SettingsService { return settingsService{e.d} }

type settingsService struct{ Deps }

func (s settingsService) Get(ctx context.Context, _ *api.SettingsGetParams) (*api.Settings, error) {
	st, err := s.DB.Settings(ctx)
	if err != nil {
		return nil, err
	}
	return s.answer(ctx, st)
}

// answer is the settings as the API gives them: favorites since deleted
// left out.
func (s settingsService) answer(ctx context.Context, st store.Settings) (*api.Settings, error) {
	mbs, err := s.DB.ListMailboxes(ctx, 0)
	if err != nil {
		return nil, err
	}
	exists := map[int64]bool{}
	for _, mb := range mbs {
		exists[mb.ID] = true
	}
	st.Favorites = slices.DeleteFunc(slices.Clone(st.Favorites), func(id int64) bool { return !exists[id] })
	return toAPISettings(st), nil
}

// maxFavorites bounds the sidebar's Favorites.
const maxFavorites = 50

// maxFlagName bounds a flag's name, in characters.
const maxFlagName = 40

func (s settingsService) Set(ctx context.Context, p *api.SettingsSetParams) (*api.Settings, error) {
	var out store.Settings
	err := s.DB.Tx(ctx, func(tx *store.Tx) error {
		st, err := s.DB.Settings(ctx)
		if err != nil {
			return err
		}
		if p.UndoDelay != nil {
			if !slices.Contains([]int64{0, 10, 20, 30}, *p.UndoDelay) {
				return api.InvalidParams("undoDelay must be 0, 10, 20 or 30 seconds")
			}
			st.UndoDelay = int(*p.UndoDelay)
		}
		if p.NotifyScope != nil {
			if !p.NotifyScope.Valid() {
				return api.InvalidParams("notifyScope %q is unknown", *p.NotifyScope)
			}
			st.NotifyScope, st.NotifySmartID = string(*p.NotifyScope), 0
		}
		if p.NotifySmartID != nil {
			if st.NotifyScope != string(api.NotifyScopeSmart) {
				return api.InvalidParams("notifySmartId goes with notifyScope smart")
			}
			if _, err := s.DB.SmartMailbox(ctx, *p.NotifySmartID); err != nil {
				return apiError(err, fmt.Sprintf("smart mailbox %d", *p.NotifySmartID))
			}
			st.NotifySmartID = *p.NotifySmartID
		}
		if st.NotifyScope == string(api.NotifyScopeSmart) && st.NotifySmartID == 0 {
			return api.InvalidParams("notifyScope smart needs notifySmartId")
		}
		if p.FlagNames != nil {
			names, err := flagNames(p.FlagNames)
			if err != nil {
				return err
			}
			st.FlagNames = names
		}
		if p.Favorites != nil {
			if len(p.Favorites) > maxFavorites || len(slices.Compact(slices.Sorted(slices.Values(p.Favorites)))) != len(p.Favorites) {
				return api.InvalidParams("favorites must be at most %d mailboxes, without repeats", maxFavorites)
			}
			for _, id := range p.Favorites {
				if _, err := s.DB.GetMailbox(ctx, id); err != nil {
					return apiError(err, fmt.Sprintf("mailbox %d", id))
				}
			}
			st.Favorites = slices.Clone(p.Favorites)
		}
		out = st
		return tx.SetSettings(ctx, st)
	})
	if err != nil {
		return nil, err
	}
	return s.answer(ctx, out)
}

// flagNames checks seven flag names and trims them.
func flagNames(in []string) ([]string, error) {
	if len(in) != store.FlagColors {
		return nil, api.InvalidParams("flagNames needs %d names, one per color", store.FlagColors)
	}
	out := make([]string, len(in))
	for i, n := range in {
		n = strings.TrimSpace(n)
		if utf8.RuneCountInString(n) > maxFlagName || strings.ContainsFunc(n, unicode.IsControl) {
			return nil, api.InvalidParams("flag name %d is longer than %d characters or has control characters", i+1, maxFlagName)
		}
		out[i] = n
	}
	return out, nil
}

func toAPISettings(s store.Settings) *api.Settings {
	out := &api.Settings{UndoDelay: int64(s.UndoDelay), NotifyScope: api.NotifyScope(s.NotifyScope), FlagNames: s.FlagNames,
		Favorites: s.Favorites}
	if out.Favorites == nil {
		out.Favorites = []int64{}
	}
	if s.NotifySmartID != 0 {
		out.NotifySmartID = &s.NotifySmartID
	}
	return out
}

// Vips implements the vip domain.
func (e *Engine) Vips() api.VipService { return vips{e.d} }

type vips struct{ Deps }

func (v vips) List(ctx context.Context, _ *api.VipListParams) ([]api.Vip, error) {
	rows, err := v.DB.VIPs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.Vip, 0, len(rows))
	for _, r := range rows {
		vip := api.Vip{Address: r.Address, Name: r.Name}
		if r.PersonID != 0 {
			vip.PersonID = &r.PersonID
		}
		out = append(out, vip)
	}
	return out, nil
}

func (v vips) Add(ctx context.Context, p *api.VipAddParams) ([]api.Vip, error) {
	addrs, err := v.vipAddresses(ctx, p.Addresses, p.PersonID)
	if err != nil {
		return nil, err
	}
	if err := v.DB.Tx(ctx, func(tx *store.Tx) error { return tx.AddVIPs(ctx, addrs) }); err != nil {
		return nil, err
	}
	return v.List(ctx, &api.VipListParams{})
}

func (v vips) Remove(ctx context.Context, p *api.VipRemoveParams) error {
	addrs, err := v.vipAddresses(ctx, p.Addresses, p.PersonID)
	if err != nil {
		return err
	}
	return v.DB.Tx(ctx, func(tx *store.Tx) error { return tx.RemoveVIPs(ctx, addrs) })
}

// vipAddresses are the addresses given and a person's, lowercased.
func (v vips) vipAddresses(ctx context.Context, addresses []string, personID *int64) ([]string, error) {
	if len(addresses) == 0 && personID == nil {
		return nil, api.InvalidParams("give addresses or a personId")
	}
	var out []string
	for _, a := range addresses {
		a = strings.ToLower(strings.TrimSpace(a))
		if !strings.Contains(a, "@") || strings.ContainsFunc(a, unicode.IsSpace) {
			return nil, api.InvalidParams("%q is not an email address", a)
		}
		out = append(out, a)
	}
	if personID != nil {
		emails, err := v.DB.PersonEmails(ctx, *personID)
		if err != nil {
			return nil, apiError(err, fmt.Sprintf("person %d", *personID))
		}
		out = append(out, emails...)
	}
	return out, nil
}

// Smart implements the smart domain.
func (e *Engine) Smart() api.SmartService { return smartMailboxes{e.d} }

type smartMailboxes struct{ Deps }

// maxSmartName bounds a smart mailbox's name, in characters.
const maxSmartName = 100

func (s smartMailboxes) List(ctx context.Context, _ *api.SmartListParams) ([]api.SmartMailbox, error) {
	rows, err := s.DB.SmartMailboxes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.SmartMailbox, 0, len(rows))
	for _, r := range rows {
		m, err := s.toAPI(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (s smartMailboxes) Create(ctx context.Context, p *api.SmartCreateParams) (*api.SmartMailbox, error) {
	name, err := smartName(p.Name)
	if err != nil {
		return nil, err
	}
	if err := store.CheckConditions(p.Conditions); err != nil {
		return nil, api.InvalidParams("%v", err)
	}
	in := store.SmartMailbox{Name: name, Conditions: p.Conditions,
		IncludeTrash: p.IncludeTrash != nil && *p.IncludeTrash, IncludeSent: p.IncludeSent != nil && *p.IncludeSent}
	var out store.SmartMailbox
	err = s.DB.Tx(ctx, func(tx *store.Tx) error {
		var err error
		out, err = tx.CreateSmartMailbox(ctx, in)
		return err
	})
	if err != nil {
		return nil, err
	}
	m, err := s.toAPI(ctx, out)
	return &m, err
}

func (s smartMailboxes) Update(ctx context.Context, p *api.SmartUpdateParams) (*api.SmartMailbox, error) {
	cur, err := s.DB.SmartMailbox(ctx, p.ID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("smart mailbox %d", p.ID))
	}
	if p.Name != nil {
		if cur.Name, err = smartName(*p.Name); err != nil {
			return nil, err
		}
	}
	if p.Conditions != nil {
		if err := store.CheckConditions(*p.Conditions); err != nil {
			return nil, api.InvalidParams("%v", err)
		}
		cur.Conditions = *p.Conditions
	}
	if p.IncludeTrash != nil {
		cur.IncludeTrash = *p.IncludeTrash
	}
	if p.IncludeSent != nil {
		cur.IncludeSent = *p.IncludeSent
	}
	var out store.SmartMailbox
	err = s.DB.Tx(ctx, func(tx *store.Tx) error {
		var err error
		out, err = tx.UpdateSmartMailbox(ctx, cur)
		return err
	})
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("smart mailbox %d", p.ID))
	}
	m, err := s.toAPI(ctx, out)
	return &m, err
}

func (s smartMailboxes) Delete(ctx context.Context, p *api.SmartDeleteParams) error {
	err := s.DB.Tx(ctx, func(tx *store.Tx) error { return tx.DeleteSmartMailbox(ctx, p.ID) })
	return apiError(err, fmt.Sprintf("smart mailbox %d", p.ID))
}

func (s smartMailboxes) Move(ctx context.Context, p *api.SmartMoveParams) error {
	if p.Position < 0 {
		return api.InvalidParams("position must not be negative")
	}
	err := s.DB.Tx(ctx, func(tx *store.Tx) error { return tx.MoveSmartMailbox(ctx, p.ID, int(p.Position)) })
	return apiError(err, fmt.Sprintf("smart mailbox %d", p.ID))
}

func (s smartMailboxes) FromSearch(ctx context.Context, p *api.SmartFromSearchParams) (*api.Conditions, error) {
	c := search.ToConditions(search.Parse(p.Text, time.Now(), time.Local), time.Local)
	if p.MailboxID != nil {
		mbs, err := s.DB.ListMailboxes(ctx, 0)
		if err != nil {
			return nil, err
		}
		if !slices.ContainsFunc(mbs, func(mb store.Mailbox) bool { return mb.ID == *p.MailboxID }) {
			return nil, api.NotFound("mailbox %d does not exist", *p.MailboxID)
		}
		c.Conditions = append(c.Conditions, api.Condition{Field: api.ConditionFieldMailbox, Op: api.ConditionOpIs,
			Value: strconv.FormatInt(*p.MailboxID, 10)})
	}
	return &c, nil
}

// toAPI is a smart mailbox with its unread count.
func (s smartMailboxes) toAPI(ctx context.Context, m store.SmartMailbox) (api.SmartMailbox, error) {
	_, unread, err := s.DB.CountView(ctx, store.ViewFilter{SmartMailboxID: m.ID})
	if err != nil {
		return api.SmartMailbox{}, err
	}
	return api.SmartMailbox{ID: m.ID, Name: m.Name, Position: int64(m.Position), Conditions: m.Conditions,
		IncludeTrash: m.IncludeTrash, IncludeSent: m.IncludeSent, Unread: int64(unread)}, nil
}

// smartName checks a smart mailbox's name and trims it.
func smartName(n string) (string, error) {
	n = strings.TrimSpace(n)
	if n == "" || utf8.RuneCountInString(n) > maxSmartName || strings.ContainsFunc(n, unicode.IsControl) {
		return "", api.InvalidParams("a smart mailbox's name must be 1 to %d characters without control characters", maxSmartName)
	}
	return n, nil
}

// Rules implements the rule domain.
func (e *Engine) Rules() api.RuleService { return rules{e.d} }

type rules struct{ Deps }

// maxRuleActions bounds one rule's actions.
const maxRuleActions = 20

func (r rules) List(ctx context.Context, _ *api.RuleListParams) ([]api.Rule, error) {
	rows, err := r.DB.Rules(ctx)
	if err != nil {
		return nil, err
	}
	mailboxes, err := r.mailboxIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.Rule, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAPIRule(row, mailboxes))
	}
	return out, nil
}

func (r rules) Create(ctx context.Context, p *api.RuleCreateParams) (*api.Rule, error) {
	in := store.Rule{Enabled: p.Enabled == nil || *p.Enabled, Conditions: p.Conditions, Actions: p.Actions}
	var err error
	if in.Name, err = smartName(p.Name); err != nil {
		return nil, api.InvalidParams("a rule's name must be 1 to %d characters without control characters", maxSmartName)
	}
	if err := r.check(ctx, in); err != nil {
		return nil, err
	}
	var out store.Rule
	err = r.DB.Tx(ctx, func(tx *store.Tx) error {
		var err error
		out, err = tx.CreateRule(ctx, in)
		return err
	})
	if err != nil {
		return nil, err
	}
	return r.answer(ctx, out)
}

func (r rules) Update(ctx context.Context, p *api.RuleUpdateParams) (*api.Rule, error) {
	cur, err := r.DB.Rule(ctx, p.ID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("rule %d", p.ID))
	}
	if p.Name != nil {
		if cur.Name, err = smartName(*p.Name); err != nil {
			return nil, api.InvalidParams("a rule's name must be 1 to %d characters without control characters", maxSmartName)
		}
	}
	if p.Conditions != nil {
		cur.Conditions = *p.Conditions
	}
	if p.Actions != nil {
		cur.Actions = p.Actions
	}
	if p.Enabled != nil {
		cur.Enabled = *p.Enabled
	}
	// Only what changes is checked, so a rule whose mailbox is gone can
	// still be renamed or turned off.
	if p.Conditions != nil {
		if err := store.CheckConditions(cur.Conditions); err != nil {
			return nil, api.InvalidParams("%v", err)
		}
	}
	if p.Actions != nil {
		if err := r.checkActions(ctx, cur.Actions); err != nil {
			return nil, err
		}
	}
	var out store.Rule
	err = r.DB.Tx(ctx, func(tx *store.Tx) error {
		var err error
		out, err = tx.UpdateRule(ctx, cur)
		return err
	})
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("rule %d", p.ID))
	}
	return r.answer(ctx, out)
}

func (r rules) Delete(ctx context.Context, p *api.RuleDeleteParams) error {
	err := r.DB.Tx(ctx, func(tx *store.Tx) error { return tx.DeleteRule(ctx, p.ID) })
	return apiError(err, fmt.Sprintf("rule %d", p.ID))
}

func (r rules) Move(ctx context.Context, p *api.RuleMoveParams) error {
	if p.Position < 0 {
		return api.InvalidParams("position must not be negative")
	}
	err := r.DB.Tx(ctx, func(tx *store.Tx) error { return tx.MoveRule(ctx, p.ID, int(p.Position)) })
	return apiError(err, fmt.Sprintf("rule %d", p.ID))
}

func (r rules) Apply(ctx context.Context, p *api.RuleApplyParams) (*api.RuleApplied, error) {
	if r.Sync == nil {
		return nil, api.Unavailable("sync is not running")
	}
	n, err := r.Sync.ApplyRules(ctx, p.IDs)
	if err != nil {
		return nil, opError(err)
	}
	return &api.RuleApplied{Matched: int64(n)}, nil
}

// check refuses a rule maild cannot run: conditions that do not compile,
// and actions that are unknown, lack what they need, or name a mailbox
// that does not exist.
func (r rules) check(ctx context.Context, in store.Rule) error {
	if err := store.CheckConditions(in.Conditions); err != nil {
		return api.InvalidParams("%v", err)
	}
	return r.checkActions(ctx, in.Actions)
}

// checkActions refuses actions that are unknown, lack what they need, or
// name a mailbox that does not exist.
func (r rules) checkActions(ctx context.Context, actions []api.RuleAction) error {
	if len(actions) == 0 || len(actions) > maxRuleActions {
		return api.InvalidParams("a rule needs 1 to %d actions", maxRuleActions)
	}
	mailboxes, err := r.mailboxIDs(ctx)
	if err != nil {
		return err
	}
	for i, a := range actions {
		n := i + 1
		if !a.Kind.Valid() {
			return api.InvalidParams("action %d: %q is not an action", n, a.Kind)
		}
		needsMailbox := a.Kind == api.RuleActionKindMove || a.Kind == api.RuleActionKindCopy
		switch {
		case needsMailbox && a.MailboxID == nil:
			return api.InvalidParams("action %d: %s needs a mailboxId", n, a.Kind)
		case needsMailbox && !mailboxes[*a.MailboxID]:
			return api.NotFound("action %d: mailbox %d does not exist", n, *a.MailboxID)
		case !needsMailbox && a.MailboxID != nil:
			return api.InvalidParams("action %d: %s takes no mailboxId", n, a.Kind)
		case a.Kind == api.RuleActionKindFlag && (a.Color == nil || *a.Color < 1 || *a.Color > 7):
			return api.InvalidParams("action %d: flag needs a color 1-7", n)
		case a.Kind != api.RuleActionKindFlag && a.Color != nil:
			return api.InvalidParams("action %d: %s takes no color", n, a.Kind)
		}
	}
	return nil
}

func (r rules) answer(ctx context.Context, row store.Rule) (*api.Rule, error) {
	mailboxes, err := r.mailboxIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := toAPIRule(row, mailboxes)
	return &out, nil
}

// mailboxIDs is every mailbox's ID, for checking actions.
func (r rules) mailboxIDs(ctx context.Context) (map[int64]bool, error) {
	mbs, err := r.DB.ListMailboxes(ctx, 0)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]bool, len(mbs))
	for _, mb := range mbs {
		out[mb.ID] = true
	}
	return out, nil
}

// toAPIRule is a rule with the problem of an action whose mailbox is gone.
func toAPIRule(r store.Rule, mailboxes map[int64]bool) api.Rule {
	out := api.Rule{ID: r.ID, Name: r.Name, Position: int64(r.Position), Enabled: r.Enabled,
		Conditions: r.Conditions, Actions: r.Actions}
	if out.Actions == nil {
		out.Actions = []api.RuleAction{}
	}
	for i, a := range r.Actions {
		if a.MailboxID != nil && !mailboxes[*a.MailboxID] {
			problem := fmt.Sprintf("action %d: mailbox %d is gone", i+1, *a.MailboxID)
			out.Problem = &problem
			break
		}
	}
	return out
}

// Remind sets or clears Remind Me reminders (ADR-0025): at a time in the
// future, a message comes back to the top of its account's inbox.
func (m messages) Remind(ctx context.Context, p *api.MessageRemindParams) error {
	if p.At != nil && !p.At.After(m.DB.Now()) {
		return api.InvalidParams("a reminder's time must be in the future")
	}
	rows, err := m.DB.Summaries(ctx, p.IDs)
	if err != nil {
		return err
	}
	if len(rows) != len(slices.Compact(slices.Sorted(slices.Values(p.IDs)))) {
		return api.NotFound("a message does not exist")
	}
	byAccount := map[int64][]int64{}
	for _, r := range rows {
		byAccount[r.AccountID] = append(byAccount[r.AccountID], r.ID)
	}
	for id := range byAccount {
		acct, err := m.DB.GetAccount(ctx, id)
		if err != nil {
			return err
		}
		if acct.ReadOnly {
			return api.Conflict("account %d is read-only", id)
		}
	}
	return m.DB.Tx(ctx, func(tx *store.Tx) error {
		if p.At != nil {
			if err := tx.SetMessageReminders(ctx, p.IDs, *p.At); err != nil {
				return err
			}
		} else if _, err := tx.ClearMessageReminders(ctx, p.IDs); err != nil {
			return err
		}
		for _, acct := range slices.Sorted(maps.Keys(byAccount)) {
			if err := tx.Emit(ctx, api.MessageChanged{AccountID: acct, IDs: byAccount[acct]}); err != nil {
				return err
			}
		}
		return nil
	})
}

// Reschedule gives a Send Later message a new time and rebuilds it with
// that Date (ADR-0025); a time within the undo delay sends it after the
// undo delay, as draft.send would.
func (o outbox) Reschedule(ctx context.Context, p *api.OutboxRescheduleParams) (*api.OutboxItem, error) {
	it, err := o.DB.GetOutbox(ctx, p.ID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("outbox message %d", p.ID))
	}
	if it.State != "queued" || !it.Scheduled || it.DraftID == 0 {
		return nil, api.Conflict("message %d is not waiting to be sent later", p.ID)
	}
	dr, err := o.DB.GetDraft(ctx, it.DraftID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("draft %d", it.DraftID))
	}
	delay, err := o.undoDelay(ctx)
	if err != nil {
		return nil, err
	}
	when := sendTime(o.DB.Now(), delay, &p.SendAt)
	b, err := o.build(ctx, dr, when.date)
	if err != nil {
		return nil, err
	}
	var out store.OutboxItem
	err = o.DB.Tx(ctx, func(tx *store.Tx) error {
		err := tx.RescheduleOutbox(ctx, it.ID, when.at, when.scheduled, b.blobID)
		if errors.Is(err, store.ErrConflict) {
			return api.Conflict("message %d is already being sent", p.ID)
		}
		if err != nil {
			return err
		}
		if out, err = tx.GetOutbox(ctx, it.ID); err != nil {
			return err
		}
		return tx.Emit(ctx, api.OutboxChanged{ID: out.ID, AccountID: out.AccountID, State: api.OutboxStateQueued})
	})
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("outbox message %d", p.ID))
	}
	if o.Sync != nil {
		o.Sync.OutboxChanged(out.AccountID)
	}
	r := toAPIOutbox(out)
	return &r, nil
}
