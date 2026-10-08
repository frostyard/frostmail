package store

import (
	"context"
	"time"
)

// RecordAddresses counts addresses seen in mail. Task T-0038 implements it;
// the stub records nothing.
func (t *Tx) RecordAddresses(_ context.Context, _ []Address, _ time.Time) error {
	return nil
}

// SuggestAddresses completes a typed prefix. Task T-0038 implements it; the
// stub finds nothing.
func (d *DB) SuggestAddresses(_ context.Context, _ string, _ int) ([]Address, error) {
	return nil, nil
}
