// Package config is the only door to the environment.
//
// Everything this binary reads from outside itself is named here, once, so that
// the list of knobs is a file somebody can read rather than a grep. An
// architecture test fails the build if any other package calls os.Getenv, which
// is what keeps that promise true a year from now.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/mail"
)

// Config is the whole of what the environment can say to this binary.
type Config struct {
	// Addr is the listening address, host optional: ":8080" or "127.0.0.1:8080".
	Addr string

	// DBFile is the SQLite file. The directory must exist.
	DBFile string

	// LogJSON switches the logger from text to JSON, for whoever collects it.
	LogJSON bool

	// TrustProxy says a reverse proxy sits in front and X-Forwarded-For may be
	// read. Off by default: with it on and no proxy actually there, the caller
	// gets to choose the address we attribute their requests to.
	TrustProxy bool

	// TrustProxyHops is how many proxies sit in front. The address we trust is
	// the one *our* proxy appended, which is this many places from the right of
	// X-Forwarded-For — never the leftmost, which the caller writes.
	TrustProxyHops int

	// KeyFile is where a version before this one kept the key that encrypted
	// the forge credentials. Secrets are no longer encrypted at rest, so this
	// is read exactly once — at startup, to move what is still sealed into the
	// plain columns beside it — and never written. It stays a setting because
	// an operator who moved the key is the one operator whose values would
	// otherwise become unreadable at the moment of upgrading.
	KeyFile string

	// PublicURL is where this instance is reachable from outside, used to build
	// the links in notifications and follow-up mail. Nothing breaks without it;
	// the links are simply omitted, which is worse and honest.
	PublicURL string

	// Mail is the optional relay. With no host configured the notify-me
	// checkbox never appears, rather than promising mail that will not arrive.
	Mail mail.Config

	// Offline stops every outbound connection this binary can make. It exists
	// so the claim in SECURITY.md — that the only connections are the forge and
	// the notification channel the operator configured — is something an
	// operator can verify rather than take on trust: turn this on, and if
	// anything still reaches the network, the document is wrong.
	Offline bool
}

// Load reads the environment. It never fails: every value has a working
// default, because a server that refuses to start over an unset variable is a
// server that gets started with the variable set to something wrong.
func Load() Config {
	return Config{
		Addr:           addr(),
		DBFile:         str("LASTHELP_DB_FILE", "lasthelp.db"),
		KeyFile:        keyFile(),
		LogJSON:        strings.EqualFold(str("LASTHELP_LOG_FORMAT", "text"), "json"),
		TrustProxy:     boolean("LASTHELP_TRUST_PROXY", false),
		TrustProxyHops: hops(),
		Offline:        boolean("LASTHELP_OFFLINE", false),
		PublicURL:      str("LASTHELP_PUBLIC_URL", ""),
		Mail: mail.Config{
			Host:     str("LASTHELP_SMTP_HOST", ""),
			Port:     number("LASTHELP_SMTP_PORT", 587),
			Username: str("LASTHELP_SMTP_USERNAME", ""),
			Password: str("LASTHELP_SMTP_PASSWORD", ""),
			From:     str("LASTHELP_SMTP_FROM", ""),
			StartTLS: boolean("LASTHELP_SMTP_STARTTLS", true),
		},
	}
}

// addr accepts either a bare port or a full address, because both are what
// people type, and normalises to what net/http wants.
func addr() string {
	raw := str("LASTHELP_ADDR", "")
	if raw == "" {
		raw = str("LASTHELP_PORT", "8080")
	}
	if !strings.Contains(raw, ":") {
		return ":" + raw
	}
	return raw
}

// hops is at least one: zero or a negative would mean reading the entry our own
// proxy appended is impossible, and the safe reading of a broken setting is the
// normal one.
func hops() int {
	if n, err := strconv.Atoi(str("LASTHELP_TRUST_PROXY_HOPS", "")); err == nil && n >= 1 {
		return n
	}
	return 1
}

func str(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func boolean(key string, fallback bool) bool {
	v, err := strconv.ParseBool(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return v
}

// keyFile defaults to sitting next to the database, because that is where the
// version that created it put it. Nothing creates it now: a missing file means
// this installation never encrypted anything, which is the normal case.
func keyFile() string {
	if explicit := str("LASTHELP_KEY_FILE", ""); explicit != "" {
		return explicit
	}
	db := str("LASTHELP_DB_FILE", "lasthelp.db")
	return filepath.Join(filepath.Dir(db), "lasthelp.key")
}

func number(key string, fallback int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return fallback
}
