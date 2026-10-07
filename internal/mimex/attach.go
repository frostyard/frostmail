package mimex

// Attachment is a part a reader shows as a file rather than as the body.
type Attachment struct {
	Filename    string // decoded from RFC 2231 and RFC 2047 forms
	ContentType string // lowercase type/subtype
	Size        int64  // decoded bytes (raw bytes for message/rfc822)
	ContentID   string // without angle brackets
	Inline      bool   // shown within the body (cid: images)
}

// Attachments lists a raw message's attachments in tree order. Task T-0022
// implements it; the stub finds none.
func Attachments(raw []byte) ([]Attachment, error) {
	return nil, nil
}
