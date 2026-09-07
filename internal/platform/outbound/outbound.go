// Package outbound is the one way out.
//
// Every connection this binary makes to somebody else's server goes through
// here: forge APIs, forge webhook deliveries we fetch back, notification
// channels. SECURITY.md promises there is nothing else, and an architecture
// test fails the build if any other package so much as constructs an
// http.Client — because the way a promise like that stops being true is not
// malice, it is somebody adding a feature that needs to fetch one small thing.
//
// # Private addresses are allowed here, on purpose
//
// The usual advice is to refuse private, loopback and link-local addresses.
// That advice assumes the URL came from a stranger. Here it came from the
// operator: pointing at a Gitea on 192.168.1.20, or at a Forgejo on the same
// host, is the feature. Refusing it would break the product for exactly the
// people it is built for.
//
// What is *not* allowed is the other half of that trade. Nothing a member of
// the public writes ever becomes a URL this package fetches — no link
// previews, no remote avatars, no embeds — and the cloud metadata addresses
// stay blocked regardless, because no forge lives there and a redirect into
// one is how a LAN-reachable service becomes credentials.
package outbound

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// ErrOffline is returned when the offline switch is on.
var ErrOffline = errors.New("outbound connections are disabled (LASTHELP_OFFLINE)")

// MaxResponseBytes caps what a reply may be. A forge that answers with a
// gigabyte — compromised, misconfigured, or just paginating badly — must not be
// able to spend this server's memory.
const MaxResponseBytes = 8 << 20 // 8 MiB

// Client is the gate. Build one at startup and hand it to whoever needs to
// reach outwards; do not construct http.Client anywhere else.
type Client struct {
	http    *http.Client
	offline bool
}

// New builds the gate. offline comes from configuration and is read on every
// request rather than captured once, so that a field a caller might forget
// cannot exist.
func New(offline bool, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Client{
		offline: offline,
		http: &http.Client{
			Timeout: timeout,
			// A redirect is a server telling us to go somewhere else. Following
			// one to a different host is how a forge on the LAN turns into a
			// request to whatever that forge was told to point at.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) == 0 {
					return nil
				}
				if len(via) >= 5 {
					return errors.New("too many redirects")
				}
				if req.URL.Host != via[0].URL.Host {
					return fmt.Errorf("refusing to follow a redirect from %s to another host (%s)",
						via[0].URL.Host, req.URL.Host)
				}
				return nil
			},
		},
	}
}

// Do sends a request, or refuses to.
//
// The caller gets a body already bounded by MaxResponseBytes, so no caller has
// to remember to bound it.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if c.offline {
		return nil, ErrOffline
	}
	if err := allowed(req.URL); err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body = boundedBody{ReadCloser: resp.Body, r: io.LimitReader(resp.Body, MaxResponseBytes)}
	return resp, nil
}

type boundedBody struct {
	io.ReadCloser
	r io.Reader
}

func (b boundedBody) Read(p []byte) (int, error) { return b.r.Read(p) }

// allowed is the address policy: everything except the places no forge lives
// and every URL shape a forge never has.
func allowed(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("refusing scheme %q: only http and https go out of here", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return errors.New("refusing a URL with no host")
	}

	// A name is resolved by the dialler, later and possibly to something else.
	// Blocking by name here would be theatre; what this catches is the literal
	// address, which is how the metadata endpoints are always reached.
	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}
	if isMetadata(ip) {
		return fmt.Errorf("refusing %s: cloud metadata addresses are never a forge, "+
			"and a redirect into one is how a reachable service becomes credentials", ip)
	}
	return nil
}

// metadataAddresses are the well-known cloud instance metadata endpoints. They
// stay blocked even though private addresses are allowed: no forge is served
// from them, and reaching one returns credentials for the machine.
var metadataAddresses = []net.IP{
	net.ParseIP("169.254.169.254"), // AWS, GCP, Azure, DigitalOcean, OpenStack
	net.ParseIP("169.254.170.2"),   // AWS ECS task metadata
	net.ParseIP("fd00:ec2::254"),   // AWS IMDS over IPv6
	net.ParseIP("100.100.100.200"), // Alibaba Cloud
}

func isMetadata(ip net.IP) bool {
	for _, blocked := range metadataAddresses {
		if blocked != nil && blocked.Equal(ip) {
			return true
		}
	}
	return false
}
