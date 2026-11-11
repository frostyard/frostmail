package pimsync

import (
	"errors"
	"fmt"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/store"
)

// An error only a new sign-in cures is a sign-in entry, not a failure that
// says it retries.
func TestRecordTellsSignInFromFailure(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
		want api.ActivityKind
	}{
		{"no scope", errScope, api.ActivityKindSigninneeded},
		{"no grant", fmt.Errorf("%w: %w", errSignIn, oauth.ErrReauth), api.ActivityKindSigninneeded},
		{"wrapped no scope", fmt.Errorf("sync graph calendar: %w", errScope), api.ActivityKindSigninneeded},
		{"a refused grant, unwrapped", oauth.ErrReauth, api.ActivityKindSigninneeded},
		{"no client", oauth.ErrNoClient, api.ActivityKindSigninneeded},
		{"a refused password", davx.ErrUnauthorized, api.ActivityKindSigninneeded},
		{"anything else", errors.New("no"), api.ActivityKindFailed},
	} {
		t.Run(c.name, func(t *testing.T) {
			hub := activity.New(nil)
			p := &pass{m: &Manager{cfg: Config{Activity: hub}}, acct: store.Account{ID: 1, Kind: api.AccountKindGmail}}
			p.record(api.ServiceKindTasks, 0, 0, c.err)
			got := hub.Recent(1)
			if len(got) != 1 || got[0].Kind != c.want {
				t.Fatalf("entries = %+v, want one %s", got, c.want)
			}
			if c.want == api.ActivityKindSigninneeded && *got[0].ErrorClass != api.ErrorClassAuth {
				t.Errorf("class = %s, want auth", *got[0].ErrorClass)
			}
		})
	}
}
