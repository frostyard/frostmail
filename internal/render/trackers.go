package render

import (
	"net/url"
	"strings"
)

// trackerHosts serve only open-tracking images; matched as the host or a
// parent domain. Most tracking pixels are caught by size and visibility
// instead (trackingImage); this list covers ones that hide neither.
var trackerHosts = []string{
	"mailtrack.io",
	"ct.sendgrid.net",
	"mandrillapp.com",
}

// trackerPaths are path fragments of common open-tracking endpoints.
var trackerPaths = []string{
	"/track/open",
	"/wf/open",
	"/open.gif",
	"/open.php",
	"/pixel.gif",
	"/beacon.gif",
}

// trackerURL reports whether an image URL is a known open-tracking endpoint.
func trackerURL(u *url.URL) bool {
	host := strings.ToLower(u.Hostname())
	for _, h := range trackerHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	path := strings.ToLower(u.EscapedPath())
	for _, p := range trackerPaths {
		if strings.Contains(path, p) {
			return true
		}
	}
	return false
}
