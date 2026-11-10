package engine_test

import (
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// The erase settings (ADR-0051) are 0 (Never) on a new account, set by
// account.update, kept by updates that do not name them, and only 0, 1, 7 or
// 30 are accepted.
func TestEraseDays(t *testing.T) {
	c := rpctest.Start(t).Dial(t)
	ctx := t.Context()
	a, err := c.Account().Create(ctx, createParams("erase@mailtest.test"))
	if err != nil {
		t.Fatalf("create = %+v, %v", a, err)
	}
	if a.EraseTrashDays == nil || *a.EraseTrashDays != 0 || a.EraseJunkDays == nil || *a.EraseJunkDays != 0 {
		t.Fatalf("a new account's erase settings = %v, %v; want 0 and 0", a.EraseTrashDays, a.EraseJunkDays)
	}
	up, err := c.Account().Update(ctx, &api.AccountUpdateParams{ID: a.ID, EraseTrashDays: ptr(int64(7))})
	if err != nil || up.EraseTrashDays == nil || *up.EraseTrashDays != 7 || *up.EraseJunkDays != 0 {
		t.Fatalf("update to 7 days in Trash = %+v, %v", up, err)
	}
	got, err := c.Account().Get(ctx, &api.AccountGetParams{ID: a.ID})
	if err != nil || *got.EraseTrashDays != 7 {
		t.Errorf("account.get after the update = %+v, %v", got, err)
	}
	// An update that names neither keeps both.
	if up, err = c.Account().Update(ctx, &api.AccountUpdateParams{ID: a.ID, EraseJunkDays: ptr(int64(30))}); err != nil ||
		*up.EraseTrashDays != 7 || *up.EraseJunkDays != 30 {
		t.Fatalf("update with only eraseJunkDays = %+v, %v", up, err)
	}
	for _, bad := range []int64{2, 5, 31, -1} {
		if _, err := c.Account().Update(ctx, &api.AccountUpdateParams{ID: a.ID, EraseTrashDays: ptr(bad)}); code(err) != api.CodeInvalidParams {
			t.Errorf("eraseTrashDays %d = %v, want invalid params", bad, err)
		}
	}
	if _, err := c.Account().Update(ctx, &api.AccountUpdateParams{ID: a.ID, EraseJunkDays: ptr(int64(5))}); code(err) != api.CodeInvalidParams {
		t.Errorf("eraseJunkDays 5 = %v, want invalid params", err)
	}
}
