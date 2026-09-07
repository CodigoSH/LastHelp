package mail

import (
	"bufio"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
)

// relay is enough of an SMTP server to hold a conversation, so these tests need
// no network and no container — just a listener on loopback.
type relay struct {
	listener net.Listener
	mu       sync.Mutex
	sessions []string
}

func newRelay(t *testing.T) *relay {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	r := &relay{listener: listener}
	go r.accept()
	t.Cleanup(func() { _ = listener.Close() })
	return r
}

func (r *relay) address() string { return r.listener.Addr().String() }

func (r *relay) transcript() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.sessions, "\n")
}

func (r *relay) accept() {
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			return
		}
		go r.serve(conn)
	}
}

func (r *relay) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	var session strings.Builder
	reader := bufio.NewReader(conn)
	write := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }

	write("220 relay.example ESMTP")

	inData := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		session.WriteString(line + "\n")

		if inData {
			if line == "." {
				inData = false
				write("250 2.0.0 Ok")
			}
			continue
		}

		command := strings.ToUpper(strings.SplitN(line, " ", 2)[0])
		switch command {
		case "EHLO", "HELO":
			write("250-relay.example")
			write("250 AUTH PLAIN")
		case "MAIL", "RCPT":
			write("250 2.1.0 Ok")
		case "AUTH":
			write("235 2.7.0 Accepted")
		case "DATA":
			inData = true
			write("354 End data with <CR><LF>.<CR><LF>")
		case "QUIT":
			write("221 2.0.0 Bye")
			r.record(session.String())
			return
		default:
			write("250 2.0.0 Ok")
		}
	}
	r.record(session.String())
}

func (r *relay) record(session string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions = append(r.sessions, session)
}

func sender(r *relay) Sender {
	host, portText, _ := net.SplitHostPort(r.address())
	p := 0
	for _, c := range portText {
		p = p*10 + int(c-'0')
	}
	return Sender{Config: Config{Host: host, Port: p, From: "lasthelp@example.com"}}
}

func TestAMessageArrives(t *testing.T) {
	r := newRelay(t)

	if err := sender(r).Send("somebody@example.com", "Your report changed", "It is now confirmed."); err != nil {
		t.Fatal(err)
	}

	transcript := r.transcript()
	for _, expected := range []string{
		"MAIL FROM:<lasthelp@example.com>",
		"RCPT TO:<somebody@example.com>",
		"Subject: Your report changed",
		"Content-Type: text/plain; charset=utf-8",
		"It is now confirmed.",
	} {
		if !strings.Contains(transcript, expected) {
			t.Errorf("the conversation is missing %q:\n%s", expected, transcript)
		}
	}
}

// TestRepliesGoNowhereAndSaySo.
//
// Nobody reads the address this comes from. Auto-Submitted is what stops mail
// clients threading it with real conversation and what tells an out-of-office
// autoresponder not to answer — two autoresponders answering each other is a
// loop that has taken down mail servers.
func TestItIsMarkedAsAutomatic(t *testing.T) {
	r := newRelay(t)
	if err := sender(r).Send("a@example.com", "s", "b"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.transcript(), "Auto-Submitted: auto-generated") {
		t.Error("the message is not marked as automatic")
	}
}

// TestANonASCIISubjectIsEncoded.
//
// An item title can be in any of the twenty languages. A raw UTF-8 subject line
// is not valid in a header: some relays pass it and some mangle it, and the
// ones that mangle it do so without saying.
func TestANonASCIISubjectIsEncoded(t *testing.T) {
	r := newRelay(t)
	if err := sender(r).Send("a@example.com", "لوحة الإدارة", "b"); err != nil {
		t.Fatal(err)
	}

	transcript := r.transcript()
	if !strings.Contains(transcript, "Subject: =?UTF-8?B?") {
		t.Errorf("an Arabic subject was not encoded:\n%s", transcript)
	}
	if strings.Contains(transcript, "Subject: لوحة") {
		t.Error("the raw UTF-8 subject went out on the wire")
	}
}

// TestHeaderInjectionIsRefused.
//
// The subject is an item title, written by a stranger. A line break in it ends
// the Subject header and starts another — which is how one message becomes a
// relay for somebody else's.
func TestHeaderInjectionIsRefused(t *testing.T) {
	r := newRelay(t)
	s := sender(r)

	if err := s.Send("a@example.com", "innocent\r\nBcc: everybody@example.com", "b"); err == nil {
		t.Error("a subject with a line break was sent")
	}
	if err := s.Send("a@example.com\r\nBcc: everybody@example.com", "s", "b"); err == nil {
		t.Error("an address with a line break was sent")
	}
	if transcript := r.transcript(); strings.Contains(transcript, "Bcc") {
		t.Errorf("an injected header reached the relay:\n%s", transcript)
	}
}

// TestNoRelayIsNotAnError.
//
// SMTP is optional. A portal with none configured never offers the notify-me
// checkbox, and must not fail anywhere just because it exists.
func TestNoRelayIsNotAFailure(t *testing.T) {
	var s Sender
	if err := s.Send("a@example.com", "s", "b"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("got %v, want ErrNotConfigured", err)
	}
}

func TestTheOfflineSwitchStopsMail(t *testing.T) {
	r := newRelay(t)
	s := sender(r)
	s.Offline = true

	if err := s.Send("a@example.com", "s", "b"); !errors.Is(err, ErrOffline) {
		t.Errorf("got %v, want ErrOffline", err)
	}
	if r.transcript() != "" {
		t.Error("something reached the relay while offline")
	}
}
