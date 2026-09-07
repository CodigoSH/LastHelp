// Command sign signs a checksum file with an ed25519 key.
//
// It exists so a release can be verified as having come from our CI. It is a
// separate binary from the server on purpose and an architecture rule keeps it
// out of what ships: a signing tool inside the released binary is a signing
// tool on every user's machine.
//
// Only the standard library — no GPG, no cosign, nothing to install and nothing
// whose absence breaks a release two years from now.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	switch os.Args[1] {
	case "keygen":
		keygen()
	case "sign":
		if len(os.Args) != 3 {
			usage()
		}
		sign(os.Args[2])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  sign keygen          generate a key pair")
	fmt.Fprintln(os.Stderr, "  sign sign <file>     write <file>.sig, key from RELEASE_SIGNING_KEY")
	os.Exit(2)
}

func keygen() {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		fail("generating a key: %v", err)
	}

	// Both keys are base64 and both end in "=", so the only thing telling them
	// apart used to be a paragraph of prose above each one — and by the time you
	// reached the key you had lost track of which was which. The operator who
	// asked for this said so. Each key now carries its label on its own line,
	// directly above it, and the explanation comes after both.
	fmt.Println()
	fmt.Println("=== SECRET — the CI secret RELEASE_SIGNING_KEY (the long one) ===")
	fmt.Println(base64.StdEncoding.EncodeToString(private))
	fmt.Println()
	fmt.Println("=== PUBLIC — goes in the repository (the short one) ===")
	fmt.Println(base64.StdEncoding.EncodeToString(public))
	fmt.Println()
	fmt.Println("What to do with them:")
	fmt.Println()
	fmt.Println("  SECRET  Settings → Actions → Secrets, as RELEASE_SIGNING_KEY.")
	fmt.Println("          Never in this repository, never on the download server.")
	fmt.Println("          Keep a copy somewhere safe: losing it means you cannot")
	fmt.Println("          sign with this key again, and rotation takes three releases.")
	fmt.Println()
	fmt.Println("  PUBLIC  APPEND it to BOTH of these, or half the release is unverifiable:")
	fmt.Println("            internal/version/keys.go  ReleaseKeys   so the binary can update itself")
	fmt.Println("            install.sh                RELEASE_KEYS  so a first install works")
	fmt.Println()
	fmt.Println("          Append, never replace: a binary verifies only with the keys it")
	fmt.Println("          was built with, so replacing leaves everybody on an older")
	fmt.Println("          version unable to verify. `make test` fails if you do only one.")
}

func sign(path string) {
	encoded := os.Getenv("RELEASE_SIGNING_KEY")
	if encoded == "" {
		fail("RELEASE_SIGNING_KEY is not set. An unsigned release is one nobody can " +
			"verify, so this refuses rather than producing one quietly.")
	}

	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		fail("RELEASE_SIGNING_KEY is not base64: %v", err)
	}
	if len(key) != ed25519.PrivateKeySize {
		fail("RELEASE_SIGNING_KEY is %d bytes, want %d", len(key), ed25519.PrivateKeySize)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		fail("reading %s: %v", path, err)
	}

	signature := ed25519.Sign(ed25519.PrivateKey(key), content)
	out := path + ".sig"
	if err := os.WriteFile(out, []byte(base64.StdEncoding.EncodeToString(signature)+"\n"), 0o644); err != nil {
		fail("writing %s: %v", out, err)
	}
	fmt.Printf("signed %s → %s\n", path, out)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
