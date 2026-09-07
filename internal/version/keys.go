package version

// ReleaseKeys are the public halves of the keys that sign our releases.
//
// A **list**, and that is the whole point. A binary can only verify with the
// keys it was built with, so replacing one outright would leave everybody still
// running an older version unable to verify anything — which is precisely the
// people who most need to update.
//
// Rotation is therefore overlapping, in three releases:
//
//  1. Publish a version that already carries the new key alongside the old one.
//  2. Switch what CI signs with.
//  3. In a later version, drop the old key.
//
// Each entry is base64 of a 32-byte ed25519 public key. An empty list means
// this build cannot verify anything and will refuse to update, which is the
// correct behaviour: no key does not mean accept anything.
var ReleaseKeys = []string{
	// No release has been cut yet, so there is no key. The first release adds
	// the one CI signs with — and this comment stays, because the day somebody
	// replaces rather than appends is the day this list needs to explain itself.
}
