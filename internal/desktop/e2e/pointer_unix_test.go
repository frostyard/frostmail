//go:build e2e && !windows

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

// realClick is a click the display server delivers, which reaches a page:
// the automation plugin's input goes to native UI only. It needs X11 and
// xdotool, as under Xvfb.
func realClick(t *testing.T, x, y int) {
	t.Helper()
	if os.Getenv("DISPLAY") == "" || os.Getenv("GDK_BACKEND") != "x11" {
		t.Skip("a real click needs X11: run under Xvfb with GDK_BACKEND=x11")
	}
	path, err := exec.LookPath("xdotool")
	if err != nil {
		t.Skip("a real click needs xdotool")
	}
	cmd := exec.Command(path, "mousemove", fmt.Sprint(x), fmt.Sprint(y), "click", "1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("xdotool: %v\n%s", err, out)
	}
}
