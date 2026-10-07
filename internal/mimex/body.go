package mimex

import "errors"

// BodyText returns a message's readable text and whether it has an HTML
// part. Task T-0018 implements it.
func BodyText(raw []byte) (string, bool, error) {
	return "", false, errors.New("mimex: BodyText is not implemented yet (T-0018)")
}
