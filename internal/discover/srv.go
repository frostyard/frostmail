package discover

import "context"

// FromSRV reads an address's RFC 6186 SRV records. Task T-0050 implements
// it; the stub finds nothing.
func FromSRV(_ context.Context, _ Resolver, _ string) (Settings, error) {
	return Settings{}, ErrNothing
}
