package blob

import (
	"flag"
	"os"
	"testing"
)

// TestMain skips, on Windows, the contract test that checks Unix file
// modes: Windows has none (Go reports 0666), and the store's files live in
// the user's profile, whose ACL keeps others out.
func TestMain(m *testing.M) {
	if err := flag.Set("test.skip", "^TestPutOpenRoundTrip$"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
