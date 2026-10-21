package engine_test

import (
	"errors"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// TestAgentsCannotFinishSignIn checks that account.finishSignIn admits no
// agent: the pasted address is a person's (ADR-0043).
func TestAgentsCannotFinishSignIn(t *testing.T) {
	srv := rpctest.Start(t)
	s, err := api.ParseScope("account=*")
	if err != nil {
		t.Fatal(err)
	}
	agent, _ := srv.DialAs(t, api.Caller{Kind: api.CallerKindAgent, Token: "claude", Scope: s})
	_, err = agent.Account().FinishSignIn(t.Context(), &api.AccountFinishSignInParams{
		URL: "http://localhost:1/?code=x&state=y",
	})
	if !errors.Is(err, api.ErrMethodNotFound) {
		t.Errorf("account.finishSignIn as an agent = %v, want methodNotFound", err)
	}
}
