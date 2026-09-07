package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheInstallerCanVerifyWhatWeSign.
//
// Two implementations have to agree: this signer, in Go, and install.sh, which
// verifies with openssl on somebody else's machine. Each one's own tests would
// pass with both of them wrong in the same way, and the failure mode is the
// worst kind — every release verifies here and none of them installs.
//
// So this signs with the real code and verifies with the exact command the
// installer runs, including the DER prefix trick that lets openssl read a key
// we ship as plain base64.
func TestTheInstallerCanVerifyWhatWeSign(t *testing.T) {
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("openssl is not on this machine; the installer needs it and this cannot check without it")
	}

	// Verifying an ed25519 signature over raw data needs `pkeyutl -rawin`,
	// which arrived in OpenSSL 3.0; 1.1.1 cannot do it from the command line at
	// all. On such a machine this test can prove nothing, so it says that
	// rather than failing as though the signing code were wrong — which is
	// exactly how it read the first time CI ran it on an old runner.
	version, _ := exec.Command(openssl, "version").Output()
	if !strings.HasPrefix(strings.TrimSpace(string(version)), "OpenSSL 3") {
		t.Skipf("this machine has %s; verifying ed25519 from the command line needs "+
			"OpenSSL 3.0, and install.sh refuses on anything older",
			strings.TrimSpace(string(version)))
	}

	dir := t.TempDir()
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	sums := filepath.Join(dir, "SHA256SUMS")
	if err := os.WriteFile(sums, []byte("deadbeef  lasthelp-linux-amd64\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Signed by the code the release workflow runs.
	t.Setenv("RELEASE_SIGNING_KEY", base64.StdEncoding.EncodeToString(private))
	sign(sums)

	rawSig, err := os.ReadFile(sums + ".sig")
	if err != nil {
		t.Fatal(err)
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(rawSig)))
	if err != nil {
		t.Fatal(err)
	}
	sigFile := filepath.Join(dir, "signature.bin")
	if err := os.WriteFile(sigFile, signature, 0o644); err != nil {
		t.Fatal(err)
	}

	// The key, in the shape install.sh builds: a fixed 12-byte DER
	// SubjectPublicKeyInfo prefix followed by the 32 raw bytes.
	prefix, err := base64.StdEncoding.DecodeString("MCowBQYDK2VwAyEA")
	if err != nil {
		t.Fatal(err)
	}
	keyDER := filepath.Join(dir, "key.der")
	if err := os.WriteFile(keyDER, append(prefix, public...), 0o644); err != nil {
		t.Fatal(err)
	}

	keyPEM := filepath.Join(dir, "key.pem")
	if out, err := exec.Command(openssl, "pkey", "-pubin", "-inform", "DER",
		"-in", keyDER, "-out", keyPEM).CombinedOutput(); err != nil {
		t.Fatalf("openssl could not read the key install.sh builds: %v\n%s", err, out)
	}

	// CombinedOutput, not Run: this test once failed on a machine that was not
	// this one and said only "exit status 1", which is every reason openssl can
	// have for refusing collapsed into one. What openssl actually said is the
	// whole diagnosis.
	verify := func(file string) ([]byte, error) {
		return exec.Command(openssl, "pkeyutl", "-verify", "-pubin", "-inkey", keyPEM,
			"-rawin", "-in", file, "-sigfile", sigFile).CombinedOutput()
	}

	t.Logf("verifying with %s", strings.TrimSpace(string(version)))

	if out, err := verify(sums); err != nil {
		t.Fatalf("the installer's command could not verify a signature this code produced: %v\n"+
			"openssl said: %s\nusing: %s\n\n"+
			"Every release would verify in our tests and none of them would install.",
			err, out, strings.TrimSpace(string(version)))
	}

	// And the other direction, or the check above proves only that openssl ran.
	tampered := filepath.Join(dir, "TAMPERED")
	if err := os.WriteFile(tampered, []byte("tampered  lasthelp-linux-amd64\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := verify(tampered); err == nil {
		t.Error("the installer's command accepted a changed checksum file")
	}
}

// TestSigningRefusesWithoutAKey.
//
// An unsigned release is one nobody can verify, and one that installs anyway
// teaches people the signature is decoration. It has to fail loudly at build
// time rather than produce a release quietly.
func TestSigningRefusesWithoutAKey(t *testing.T) {
	if os.Getenv("SIGN_CRASHER") == "1" {
		t.Setenv("RELEASE_SIGNING_KEY", "")
		sign(filepath.Join(t.TempDir(), "nothing"))
		return
	}

	// sign() exits the process, so it is run as one.
	cmd := exec.Command(os.Args[0], "-test.run=TestSigningRefusesWithoutAKey")
	cmd.Env = append(os.Environ(), "SIGN_CRASHER=1", "RELEASE_SIGNING_KEY=")
	err := cmd.Run()

	if err == nil {
		t.Fatal("signing with no key succeeded")
	}
}
