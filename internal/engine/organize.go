package engine

import (
	"context"

	"github.com/frostyard/frostmail/api"
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

func (s settingsService) Get(context.Context, *api.SettingsGetParams) (*api.Settings, error) {
	return nil, notYet(2)
}

func (s settingsService) Set(context.Context, *api.SettingsSetParams) (*api.Settings, error) {
	return nil, notYet(2)
}

// Vips implements the vip domain.
func (e *Engine) Vips() api.VipService { return vips{e.d} }

type vips struct{ Deps }

func (v vips) List(context.Context, *api.VipListParams) ([]api.Vip, error) { return nil, notYet(2) }
func (v vips) Add(context.Context, *api.VipAddParams) ([]api.Vip, error)   { return nil, notYet(2) }
func (v vips) Remove(context.Context, *api.VipRemoveParams) error          { return notYet(2) }

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
