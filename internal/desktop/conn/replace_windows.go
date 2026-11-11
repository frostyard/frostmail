package conn

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// replaceAttempts and replacePause bound how long a replace waits for
// another one of the same file.
const (
	replaceAttempts = 20
	replacePause    = 25 * time.Millisecond
)

// replaceFile moves from over to. On Windows a rename that replaces a file
// fails with access denied, or a sharing violation, while another replace
// of it is under way, so it tries again for a while.
func replaceFile(from, to string) error {
	var err error
	for range replaceAttempts {
		err = os.Rename(from, to)
		if err == nil || !errors.Is(err, windows.ERROR_ACCESS_DENIED) && !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return err
		}
		time.Sleep(replacePause)
	}
	return err
}
