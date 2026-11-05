package mimex

import "errors"

// ErrParse matches every error of BodyText, BodyHTML and WalkParts that
// says the message itself cannot be parsed (a malformed header or MIME
// structure). An error that WalkParts's callback returns does not match it.
// The error's own text is unchanged.
var ErrParse = errors.New("mimex: parse")

// parseError marks err with ErrParse and keeps its text.
type parseError struct{ err error }

func (e parseError) Error() string        { return e.err.Error() }
func (e parseError) Unwrap() error        { return e.err }
func (e parseError) Is(target error) bool { return target == ErrParse }

// parseFailed marks a parse error with ErrParse; nil stays nil.
func parseFailed(err error) error {
	if err == nil {
		return nil
	}
	return parseError{err}
}
