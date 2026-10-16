package api

import (
	"context"
	"net"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// dialSocket connects to maild's named pipe (ADR-0038) at the
// identification level, which lets maild read who the client is (its
// check of the client's SID) but not act as it.
func dialSocket(ctx context.Context, path string) (net.Conn, error) {
	return winio.DialPipeAccessImpLevel(ctx, path, windows.GENERIC_READ|windows.GENERIC_WRITE, winio.PipeImpLevelIdentification)
}
