package discover

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/frostyard/frostmail/api"
)

// FromSRV reads an address's RFC 6186 SRV records, preferring the
// implicit-TLS services (imaps, submissions, RFC 8314) over their
// STARTTLS counterparts (imap, submission). It returns ErrNothing when
// neither protocol is offered, and a plain error when the address has no
// usable domain.
func FromSRV(ctx context.Context, r Resolver, email string) (Settings, error) {
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return Settings{}, fmt.Errorf("discover: address %q has no domain", email)
	}
	domain := strings.ToLower(email[at+1:])

	imap, err := lookupSRV(ctx, r, "imaps", domain, api.TLSModeTLS, email)
	if err != nil {
		return Settings{}, err
	}
	if imap == nil {
		imap, err = lookupSRV(ctx, r, "imap", domain, api.TLSModeStartTLS, email)
		if err != nil {
			return Settings{}, err
		}
	}

	smtp, err := lookupSRV(ctx, r, "submissions", domain, api.TLSModeTLS, email)
	if err != nil {
		return Settings{}, err
	}
	if smtp == nil {
		smtp, err = lookupSRV(ctx, r, "submission", domain, api.TLSModeStartTLS, email)
		if err != nil {
			return Settings{}, err
		}
	}

	if imap == nil && smtp == nil {
		return Settings{}, ErrNothing
	}
	return Settings{IMAP: imap, SMTP: smtp}, nil
}

// lookupSRV resolves one service over TCP for the domain and turns the
// best usable record into a Server, or nil when the service is not
// offered. A DNS not-found is not an error; anything else is returned.
func lookupSRV(ctx context.Context, r Resolver, service, domain string, tls api.TLSMode, user string) (*Server, error) {
	_, recs, err := r.LookupSRV(ctx, service, "tcp", domain)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return nil, nil
		}
		return nil, err
	}
	var best *net.SRV
	for _, rec := range recs {
		if !usableSRV(rec) {
			continue
		}
		if best == nil || rec.Priority < best.Priority ||
			(rec.Priority == best.Priority && rec.Weight > best.Weight) {
			best = rec
		}
	}
	if best == nil {
		return nil, nil
	}
	return &Server{
		Host:     strings.TrimSuffix(best.Target, "."),
		Port:     int(best.Port),
		TLS:      tls,
		Username: user,
	}, nil
}

// usableSRV reports whether a record names an offered server: RFC 6186
// marks a service as not offered with a target of ".".
func usableSRV(rec *net.SRV) bool {
	return rec != nil && rec.Target != "" && rec.Target != "." && rec.Port != 0
}
