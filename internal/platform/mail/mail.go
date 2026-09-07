// Package mail is the second and last way out of this binary.
//
// It is separate from internal/platform/outbound because the relay is an
// address the operator configured rather than a URL anybody typed, so it cannot
// use the guard that inspects addresses — and because SMTP is not HTTP.
//
// Both gates honour the offline switch, and the architecture rule names exactly
// these two packages as allowed to open a socket.
package mail

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// ErrOffline is returned when the offline switch is on.
var ErrOffline = errors.New("outbound connections are disabled (LASTHELP_OFFLINE)")

// ErrNotConfigured means no relay was set up. It is not a failure: SMTP is
// optional, and a portal with no mail configured simply never offers the
// notify-me checkbox.
var ErrNotConfigured = errors.New("no mail relay configured")

// Config is what an operator gives us.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string

	// StartTLS upgrades the connection after greeting. Off means the relay is
	// on localhost or inside the operator's own network, which is the common
	// case for self-hosted mail and not something to refuse.
	StartTLS bool
}

func (c Config) configured() bool { return c.Host != "" && c.From != "" }

// Sender sends. It holds no connection: a relay used a few times a day does not
// need one kept open, and a pooled SMTP connection is a thing that breaks
// quietly overnight.
type Sender struct {
	Config  Config
	Offline bool
}

// Send delivers one message.
//
// Plain text only, and deliberately: an HTML mail from a feedback portal is a
// thing that lands in spam, renders differently everywhere, and gives an
// attacker somewhere to put a link that does not say where it goes.
func (s Sender) Send(to, subject, body string) error {
	if s.Offline {
		return ErrOffline
	}
	if !s.Config.configured() {
		return ErrNotConfigured
	}
	if err := checkHeader(to); err != nil {
		return err
	}
	if err := checkHeader(subject); err != nil {
		return err
	}

	address := net.JoinHostPort(s.Config.Host, port(s.Config.Port))
	message := build(s.Config.From, to, subject, body)

	client, err := dial(address)
	if err != nil {
		return fmt.Errorf("reaching the mail relay: %w", err)
	}
	defer func() { _ = client.Close() }()

	if s.Config.StartTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(nil); err != nil {
				return fmt.Errorf("starting TLS with the relay: %w", err)
			}
		}
	}

	if s.Config.Username != "" {
		auth := smtp.PlainAuth("", s.Config.Username, s.Config.Password, s.Config.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("the mail relay refused the credentials: %w", err)
		}
	}

	if err := client.Mail(s.Config.From); err != nil {
		return fmt.Errorf("sending: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("sending to %s: %w", to, err)
	}

	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("sending: %w", err)
	}
	if _, err := writer.Write([]byte(message)); err != nil {
		return fmt.Errorf("sending: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("sending: %w", err)
	}
	return client.Quit()
}

// dial is a variable so a test can point it at a fake relay without opening a
// real socket, which is also what keeps this package's tests offline.
var dial = func(address string) (*smtp.Client, error) {
	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		return nil, err
	}
	host, _, _ := net.SplitHostPort(address)
	return smtp.NewClient(conn, host)
}

// checkHeader refuses anything that would break out of a header line.
//
// A newline in an address or a subject is header injection: everything after it
// becomes a header of its own, which is how one message turns into a relay for
// somebody else's. The subject here comes from an item title, which was written
// by a stranger.
func checkHeader(value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return errors.New("a line break in a mail header is header injection")
	}
	return nil
}

func build(from, to, subject, body string) string {
	var out strings.Builder
	fmt.Fprintf(&out, "From: %s\r\n", from)
	fmt.Fprintf(&out, "To: %s\r\n", to)
	fmt.Fprintf(&out, "Subject: %s\r\n", encodeSubject(subject))
	out.WriteString("MIME-Version: 1.0\r\n")
	out.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	out.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	// Tell every mail client not to thread this with unrelated traffic and, more
	// importantly, that replying achieves nothing: there is nobody reading this
	// address.
	out.WriteString("Auto-Submitted: auto-generated\r\n")
	out.WriteString("\r\n")
	out.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	return out.String()
}

// encodeSubject wraps a non-ASCII subject in the encoded-word form.
//
// An item title can be in any of the twenty languages, and a raw UTF-8 subject
// line is not valid in a header — some relays pass it, some mangle it, and the
// ones that mangle it do so silently.
func encodeSubject(subject string) string {
	for i := 0; i < len(subject); i++ {
		if subject[i] > 127 {
			return mimeEncode(subject)
		}
	}
	return subject
}

func port(p int) string {
	if p <= 0 {
		return "587"
	}
	return fmt.Sprint(p)
}

// mimeEncode is RFC 2047 base64 encoded-word, which is what a header carrying
// anything but ASCII has to be.
func mimeEncode(value string) string {
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(value)) + "?="
}
