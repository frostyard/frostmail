package engine

import (
	"bytes"
	"context"
	"errors"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

// errObjectChanged says the object a write patched is not the one that was
// read: a sync pass replaced or removed its source, and the write runs
// again from the top.
var errObjectChanged = errors.New("engine: the written object changed")

// writeAttempts is how many times a write runs when a sync pass changes its
// object in between.
const writeAttempts = 3

// currentObject reads an object again in the write's transaction. A sync
// pass that settled a put since read leaves its ETag and href, and the
// current object has them. An object that is gone, or whose source is not
// the one that was read, returns errObjectChanged: the write was made from
// a source that is no longer stored.
func currentObject(ctx context.Context, tx *store.Tx, read store.Object) (store.Object, error) {
	current, err := tx.GetObject(ctx, read.ID)
	if errors.Is(err, store.ErrNotFound) {
		return store.Object{}, errObjectChanged
	}
	if err != nil {
		return store.Object{}, err
	}
	if !bytes.Equal(current.Raw, read.Raw) {
		return store.Object{}, errObjectChanged
	}
	return current, nil
}

// retryWrite runs write up to writeAttempts times while it returns
// errObjectChanged. Then it returns a conflict that names what was written.
// Any other result is returned at once.
func retryWrite[T any](what string, write func() (T, error)) (T, error) {
	for range writeAttempts {
		out, err := write()
		if !errors.Is(err, errObjectChanged) {
			return out, err
		}
	}
	var zero T
	return zero, api.Conflict("%s changed while it was written; write it again", what)
}
