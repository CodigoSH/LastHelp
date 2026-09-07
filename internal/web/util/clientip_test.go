package util

import (
	"net/http"
	"testing"
)

func request(remote, forwarded string) *http.Request {
	r := &http.Request{RemoteAddr: remote, Header: http.Header{}}
	if forwarded != "" {
		r.Header.Set("X-Forwarded-For", forwarded)
	}
	return r
}

// TestTheCallerCannotChooseTheirOwnAddress is the whole point of this file.
//
// A proxy appends. So the leftmost entry is whatever the caller sent, and
// reading it hands somebody a fresh rate-limit bucket for every request they
// make. This house walked past a login lockout that way.
func TestTheCallerCannotChooseTheirOwnAddress(t *testing.T) {
	// The caller claims to be 1.1.1.1; our proxy appended the truth, 9.9.9.9.
	r := request("10.0.0.5:1234", "1.1.1.1, 9.9.9.9")

	if got := ClientIP(r, true, 1); got != "9.9.9.9" {
		t.Errorf("attributed the request to %q; the leftmost entry is written by the caller", got)
	}
}

func TestWithoutAProxyThePeerIsUsed(t *testing.T) {
	r := request("9.9.9.9:1234", "1.1.1.1")
	if got := ClientIP(r, false, 1); got != "9.9.9.9" {
		t.Errorf("got %q; with no proxy declared the header means nothing", got)
	}
}

func TestAShortChainFallsBackToThePeer(t *testing.T) {
	// Two proxies declared, one entry present: the chain is not what the
	// operator said, so nothing in the header is trustworthy.
	r := request("10.0.0.5:1234", "1.1.1.1")
	if got := ClientIP(r, true, 2); got != "10.0.0.5" {
		t.Errorf("got %q, want the peer", got)
	}
}

func TestGarbageInTheHeaderFallsBackToThePeer(t *testing.T) {
	r := request("10.0.0.5:1234", "1.1.1.1, not-an-address")
	if got := ClientIP(r, true, 1); got != "10.0.0.5" {
		t.Errorf("got %q, want the peer", got)
	}
}

func TestTwoProxies(t *testing.T) {
	r := request("10.0.0.5:1234", "1.1.1.1, 2.2.2.2, 9.9.9.9")
	if got := ClientIP(r, true, 2); got != "2.2.2.2" {
		t.Errorf("got %q; with two proxies declared the second from the right is ours", got)
	}
}

// TestIPv6IsBucketedByBlock.
//
// A single customer is routinely handed a /64. Limiting ten requests per exact
// address is limiting ten requests per attempt.
func TestIPv6IsBucketedByBlock(t *testing.T) {
	a := Bucket("2001:db8:1234:5678::1")
	b := Bucket("2001:db8:1234:5678:ffff:ffff:ffff:ffff")
	if a != b {
		t.Errorf("two addresses in the same /64 bucketed differently: %q and %q", a, b)
	}

	other := Bucket("2001:db8:1234:9999::1")
	if a == other {
		t.Errorf("two different /64 blocks share a bucket: %q", a)
	}
}

func TestIPv4IsBucketedByBlock(t *testing.T) {
	if Bucket("192.0.2.7") != Bucket("192.0.2.200") {
		t.Error("two addresses in the same /24 bucketed differently")
	}
	if Bucket("192.0.2.7") == Bucket("192.0.3.7") {
		t.Error("two different /24 blocks share a bucket")
	}
}

func TestSomethingThatIsNotAnAddressIsItsOwnBucket(t *testing.T) {
	if Bucket("unix-socket") != "unix-socket" {
		t.Error("a non-address should be left alone rather than collapsed with others")
	}
}
