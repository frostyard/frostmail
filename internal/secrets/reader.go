package secrets

import (
	"context"
	"errors"
	"fmt"
)

// ErrReadOnly is what a store from OpenReader answers to Set and Delete.
var ErrReadOnly = errors.New("secrets: read-only store")

// readOnly reads from first, and from second a key first does not hold (a
// secrets file left beside the keyring Open would have emptied). It never
// changes either, and a nil second reads from first alone.
type readOnly struct {
	first, second Store
}

var _ Store = readOnly{}

func (r readOnly) Get(ctx context.Context, key string) (string, error) {
	v, err := r.first.Get(ctx, key)
	if r.second == nil || !errors.Is(err, ErrNotFound) {
		return v, err
	}
	return r.second.Get(ctx, key)
}

func (r readOnly) Set(_ context.Context, key, _ string) error {
	return fmt.Errorf("secrets: set %s: %w", key, ErrReadOnly)
}

func (r readOnly) Delete(_ context.Context, key string) error {
	return fmt.Errorf("secrets: delete %s: %w", key, ErrReadOnly)
}
