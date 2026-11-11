//go:build e2e

package e2e

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime/quotedprintable"
	"strings"
)

// decode undoes a part's transfer encoding.
func decode(encoding string, body []byte) string {
	switch strings.ToLower(encoding) {
	case "quoted-printable":
		if b, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(body))); err == nil {
			return string(b)
		}
	case "base64":
		clean := strings.Join(strings.Fields(string(body)), "")
		if b, err := base64.StdEncoding.DecodeString(clean); err == nil {
			return string(b)
		}
	}
	return string(body)
}
