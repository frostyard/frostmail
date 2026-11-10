package activity

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"syscall"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/smtpx"
)

// httpStatus is an HTTP client's error that carries the response's status
// (davx, graph, gtasks).
type httpStatus interface {
	error
	HTTPStatus() int
}

// authRefuser is an error that says whether the server refused the
// credentials (oauth's token and XOAUTH2 errors; oauth imports this
// package, so this package cannot import it).
type authRefuser interface {
	error
	AuthRefused() bool
}

// Classify reduces err to its class, an SMTP reply code or HTTP status, and
// an IMAP response code, from its type alone: it never reads the error's
// text, which can hold whatever a server said.
func Classify(err error) (api.ErrorClass, *int64, *api.ImapCode) {
	if err == nil {
		return api.ErrorClassInternal, nil, nil
	}
	if errors.Is(err, imapx.ErrAuth) || errors.Is(err, smtpx.ErrAuth) {
		return api.ErrorClassAuth, smtpCode(err), imapCode(err)
	}
	if a, ok := errors.AsType[authRefuser](err); ok && a.AuthRefused() {
		return api.ErrorClassAuth, nil, nil
	}
	if errors.Is(err, smtpx.ErrTooLarge) {
		return api.ErrorClassQuota, smtpCode(err), nil
	}
	if code, ok := imapx.ResponseCode(err); ok {
		return imapClass(code), nil, imapEnum(code)
	}
	if code, ok := smtpx.ReplyCode(err); ok {
		c := int64(code)
		return smtpClass(code), &c, nil
	}
	if h, ok := errors.AsType[httpStatus](err); ok {
		status := h.HTTPStatus()
		c := int64(status)
		return httpClass(status), &c, nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return api.ErrorClassTimeout, nil, nil
	}
	if ne, ok := errors.AsType[net.Error](err); ok && ne.Timeout() {
		return api.ErrorClassTimeout, nil, nil
	}
	if isTLS(err) {
		return api.ErrorClassTLS, nil, nil
	}
	if isNetwork(err) {
		return api.ErrorClassNetwork, nil, nil
	}
	return api.ErrorClassInternal, nil, nil
}

func isTLS(err error) bool {
	if _, ok := errors.AsType[*tls.CertificateVerificationError](err); ok {
		return true
	}
	if _, ok := errors.AsType[tls.RecordHeaderError](err); ok {
		return true
	}
	if _, ok := errors.AsType[tls.AlertError](err); ok {
		return true
	}
	if _, ok := errors.AsType[x509.UnknownAuthorityError](err); ok {
		return true
	}
	if _, ok := errors.AsType[x509.HostnameError](err); ok {
		return true
	}
	_, ok := errors.AsType[x509.CertificateInvalidError](err)
	return ok
}

func isNetwork(err error) bool {
	if _, ok := errors.AsType[*net.OpError](err); ok {
		return true
	}
	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return true
	}
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) ||
		errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE)
}

func smtpCode(err error) *int64 {
	if code, ok := smtpx.ReplyCode(err); ok {
		c := int64(code)
		return &c
	}
	return nil
}

func imapCode(err error) *api.ImapCode {
	if code, ok := imapx.ResponseCode(err); ok {
		return imapEnum(code)
	}
	return nil
}

func imapClass(code string) api.ErrorClass {
	switch code {
	case "AUTHENTICATIONFAILED", "AUTHORIZATIONFAILED":
		return api.ErrorClassAuth
	case "OVERQUOTA", "LIMIT":
		return api.ErrorClassQuota
	case "UNAVAILABLE":
		return api.ErrorClassNetwork
	case "SERVERBUG":
		return api.ErrorClassProtocol
	default:
		return api.ErrorClassRefused
	}
}

// imapEnum names an RFC 5530 response code; "" (a NO or BAD without one)
// names none.
func imapEnum(code string) *api.ImapCode {
	var v api.ImapCode
	switch code {
	case "":
		return nil
	case "AUTHENTICATIONFAILED":
		v = api.ImapCodeAuthenticationfailed
	case "AUTHORIZATIONFAILED":
		v = api.ImapCodeAuthorizationfailed
	case "UNAVAILABLE":
		v = api.ImapCodeUnavailable
	case "OVERQUOTA":
		v = api.ImapCodeOverquota
	case "LIMIT":
		v = api.ImapCodeLimit
	case "NONEXISTENT":
		v = api.ImapCodeNonexistent
	case "ALREADYEXISTS":
		v = api.ImapCodeAlreadyexists
	case "CANNOT":
		v = api.ImapCodeCannot
	case "SERVERBUG":
		v = api.ImapCodeServerbug
	default:
		v = api.ImapCodeOther
	}
	return &v
}

func smtpClass(code int) api.ErrorClass {
	switch code {
	case 530, 534, 535:
		return api.ErrorClassAuth
	case 452, 552:
		return api.ErrorClassQuota
	case 421:
		return api.ErrorClassNetwork
	default:
		return api.ErrorClassRefused
	}
}

func httpClass(status int) api.ErrorClass {
	switch {
	case status == 401:
		return api.ErrorClassAuth
	case status == 429 || status == 507:
		return api.ErrorClassQuota
	case status == 408 || status == 504:
		return api.ErrorClassTimeout
	case status == 502 || status == 503:
		return api.ErrorClassNetwork
	case status >= 500:
		return api.ErrorClassProtocol
	default:
		return api.ErrorClassRefused
	}
}
