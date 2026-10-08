package oauth

import (
	"github.com/emersion/go-sasl"
)

// SASL returns a SASL XOAUTH2 client for IMAP and SMTP. When the server
// refuses the token it sends a challenge with the failure details; the
// client answers with an empty response, as the mechanism requires, and
// Err then reports the details.
func SASL(user, accessToken string) *XOAuth2Client {
	return &XOAuth2Client{user: user, token: accessToken}
}

// XOAuth2Client is a sasl.Client for XOAUTH2.
type XOAuth2Client struct {
	user, token string
	failure     error
}

var _ sasl.Client = (*XOAuth2Client)(nil)

// Start sends the initial response.
func (c *XOAuth2Client) Start() (string, []byte, error) {
	return "XOAUTH2", XOAuth2(c.user, c.token), nil
}

// Next records the server's failure details and answers with nothing.
func (c *XOAuth2Client) Next(challenge []byte) ([]byte, error) {
	c.failure = ParseXOAuth2Error(challenge)
	return []byte{}, nil
}

// Err is the server's failure details after a refused token, if it sent
// any.
func (c *XOAuth2Client) Err() error { return c.failure }
