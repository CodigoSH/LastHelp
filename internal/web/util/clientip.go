// Package util holds the small pieces the HTTP layer shares.
package util

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP returns the address a request should be attributed to.
//
// Behind a proxy the peer address is the proxy's, identical for everybody, so
// X-Forwarded-For is read instead — but only the part of it there is any reason
// to believe.
//
// A proxy does not replace that header, it appends to it: nginx's
// $proxy_add_x_forwarded_for puts the peer behind whatever the client already
// sent. So the leftmost entry is written by the caller, who can put anything
// there and mint a fresh rate-limit bucket for every request. This house has
// already demonstrated that by walking past a login lockout with twelve
// attempts and a rotating header.
//
// The entry to trust is the one *our own* proxy appended, which is `hops`
// places from the right. If the header is shorter than the chain the operator
// declared, the chain is not what they said it was and the peer address is used.
func ClientIP(r *http.Request, trustProxy bool, hops int) string {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	if !trustProxy {
		return peer
	}

	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded == "" {
		return peer
	}
	if hops < 1 {
		hops = 1
	}

	parts := strings.Split(forwarded, ",")
	index := len(parts) - hops
	if index < 0 {
		return peer
	}

	candidate := strings.TrimSpace(parts[index])
	if net.ParseIP(candidate) == nil {
		return peer
	}
	return candidate
}

// Bucket groups an address with its neighbours.
//
// Rate limiting on the exact address alone does nothing against IPv6: a single
// customer is routinely handed a /64, which is eighteen quintillion addresses,
// so a limit of ten per address is a limit of ten per attempt. IPv4 is the
// opposite problem — a whole office or a mobile carrier shares one address — so
// the exact address is limited generously and the block is limited on top.
func Bucket(address string) string {
	ip := net.ParseIP(address)
	if ip == nil {
		return address
	}
	if v4 := ip.To4(); v4 != nil {
		return net.IP(append([]byte(nil), v4[0], v4[1], v4[2], 0)).String() + "/24"
	}
	masked := ip.Mask(net.CIDRMask(64, 128))
	return masked.String() + "/64"
}
