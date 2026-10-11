package engine

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

// The M5 domains (docs/design/organize.md, plans/0008). Each method is
// built in the phase its error names.

// notYet is the error of a method its M5 phase has not built.
func notYet(phase int) error {
	return api.Unavailable("not implemented yet: M5 phase %d", phase)
}

// Settings implements the settings domain.
func (e *Engine) Settings() api.SettingsService { return settingsService{e.d} }

type settingsService struct{ Deps }

func (s settingsService) Get(ctx context.Context, _ *api.SettingsGetParams) (*api.Settings, error) {
	st, err := s.DB.Settings(ctx)
	if err != nil {
		return nil, err
	}
	return toAPISettings(st), nil
}

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
			if *p.NotifyScope == api.NotifyScopeSmart {
				return notYet(3)
			}
			st.NotifyScope, st.NotifySmartID = string(*p.NotifyScope), 0
		}
		if p.NotifySmartID != nil && st.NotifyScope != string(api.NotifyScopeSmart) {
			return api.InvalidParams("notifySmartId goes with notifyScope smart")
		}
		if p.FlagNames != nil {
			names, err := flagNames(p.FlagNames)
			if err != nil {
				return err
			}
			st.FlagNames = names
		}
		out = st
		return tx.SetSettings(ctx, st)
	})
	if err != nil {
		return nil, err
	}
	return toAPISettings(out), nil
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
	out := &api.Settings{UndoDelay: int64(s.UndoDelay), NotifyScope: api.NotifyScope(s.NotifyScope), FlagNames: s.FlagNames}
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

func (s smartMailboxes) List(context.Context, *api.SmartListParams) ([]api.SmartMailbox, error) {
	return nil, notYet(3)
}

func (s smartMailboxes) Create(context.Context, *api.SmartCreateParams) (*api.SmartMailbox, error) {
	return nil, notYet(3)
}

func (s smartMailboxes) Update(context.Context, *api.SmartUpdateParams) (*api.SmartMailbox, error) {
	return nil, notYet(3)
}

func (s smartMailboxes) Delete(context.Context, *api.SmartDeleteParams) error { return notYet(3) }
func (s smartMailboxes) Move(context.Context, *api.SmartMoveParams) error     { return notYet(3) }

// Rules implements the rule domain.
func (e *Engine) Rules() api.RuleService { return rules{e.d} }

type rules struct{ Deps }

func (r rules) List(context.Context, *api.RuleListParams) ([]api.Rule, error) { return nil, notYet(4) }
func (r rules) Create(context.Context, *api.RuleCreateParams) (*api.Rule, error) {
	return nil, notYet(4)
}
func (r rules) Update(context.Context, *api.RuleUpdateParams) (*api.Rule, error) {
	return nil, notYet(4)
}
func (r rules) Delete(context.Context, *api.RuleDeleteParams) error { return notYet(4) }
func (r rules) Move(context.Context, *api.RuleMoveParams) error     { return notYet(4) }
func (r rules) Apply(context.Context, *api.RuleApplyParams) (*api.RuleApplied, error) {
	return nil, notYet(4)
}

// Remind sets or clears Remind Me reminders (ADR-0025).
func (m messages) Remind(context.Context, *api.MessageRemindParams) error { return notYet(5) }

// Reschedule gives a Send Later message a new time (ADR-0025).
func (o outbox) Reschedule(context.Context, *api.OutboxRescheduleParams) (*api.OutboxItem, error) {
	return nil, notYet(5)
}
