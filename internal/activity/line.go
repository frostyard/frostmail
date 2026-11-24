package activity

import (
	"github.com/frostyard/frostmail/api"
	wire "github.com/frostyard/frostmail/internal/activity/entry"
)

// Names gives a client's display names to the activity formatter.
type Names = wire.Names

// Line formats a wire entry for the client's console.
func Line(e api.ActivityEntry, names Names) (source, text string) { return wire.Line(e, names) }
