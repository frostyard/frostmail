package secrets

import (
	"flag"
	"os"
	"testing"
)

// TestMain skips, on Windows, the contract tests that check Unix file
// modes: Windows has none (Go reports 0666). The DPAPI store's tests stand
// in for them there.
func TestMain(m *testing.M) {
	if err := flag.Set("test.skip", "^(TestFileModeAndNoLeftovers|TestFileRefusesReadableFile)$"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
