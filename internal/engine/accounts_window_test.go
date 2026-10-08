package engine_test

import (
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// The sync window (ADR-0016) is set at creation, kept by updates that do
// not name it, and bounded.
func TestSyncDays(t *testing.T) {
	c := rpctest.Start(t).Dial(t)
	ctx := t.Context()
	p := createParams("window@mailtest.test")
	p.SyncDays = ptr(int64(365))
	a, err := c.Account().Create(ctx, p)
	if err != nil || a.SyncDays != 365 {
		t.Fatalf("create = %+v, %v", a, err)
	}
	if up, err := c.Account().Update(ctx, &api.AccountUpdateParams{ID: a.ID, Notify: ptr(false)}); err != nil || up.SyncDays != 365 {
		t.Errorf("an update without syncDays = %+v, %v", up, err)
	}
	list, err := c.Account().List(ctx, &api.AccountListParams{})
	if err != nil || len(list) != 1 || list[0].SyncDays != 365 {
		t.Errorf("list = %+v, %v", list, err)
	}
	other := createParams("all@mailtest.test")
	if b, err := c.Account().Create(ctx, other); err != nil || b.SyncDays != 0 {
		t.Errorf("create without syncDays = %+v, %v; want 0, every message", b, err)
	}
	bad := createParams("bad@mailtest.test")
	bad.SyncDays = ptr(int64(-5))
	if _, err := c.Account().Create(ctx, bad); code(err) != api.CodeInvalidParams {
		t.Errorf("create with syncDays -5 = %v, want invalid params", err)
	}
}
