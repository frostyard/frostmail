package discover

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/frostyard/frostmail/api"
)

// DAVStarts are where contacts or calendar discovery may start for an
// address on a domain without a provider profile (RFC 6764): the server
// the domain's _carddavs._tcp or _caldavs._tcp SRV record names, then the
// domain itself, whose /.well-known/ path davx.Discover asks first.
func DAVStarts(ctx context.Context, r Resolver, email string, service api.ServiceKind) ([]string, error) {
	_, domain, err := splitEmail(email)
	if err != nil {
		return nil, err
	}
	if r == nil {
		r = net.DefaultResolver
	}
	srv := "caldavs"
	if service == api.ServiceKindContacts {
		srv = "carddavs"
	}
	var out []string
	s, err := lookupSRV(ctx, r, srv, domain, api.TLSModeTLS, "")
	if err != nil {
		return nil, fmt.Errorf("discover %s: %w", srv, err)
	}
	if s != nil {
		host := s.Host
		if s.Port != 443 {
			host = net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
		}
		out = append(out, "https://"+host+"/")
	}
	return append(out, "https://"+domain+"/"), nil
}
