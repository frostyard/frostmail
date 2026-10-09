package pimsync

import (
	"context"
	"errors"

	"github.com/frostyard/frostmail/internal/davx"
)

// errNotYet marks what task card T-0061 has yet to write.
var errNotYet = errors.New("pimsync: not implemented yet")

// syncDAV brings the account's collections of a kind under the client's
// home set in step with the server. Task T-0061 writes it.
func (p *pass) syncDAV(ctx context.Context, c *davx.Client, kind davx.Kind, want func(davx.Collection) bool) error {
	return errNotYet
}
