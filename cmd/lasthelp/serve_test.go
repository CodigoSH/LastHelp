package main

import (
	"net"
	"net/http"
	"syscall"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/handoff"
)

// When an update replaces the process, the server stops accepting and gives its socket away, and
// serve has to stay: returning would end the process before the exec that replaces it, and the port
// with it. Only a signal ends it from then on.
func TestServeStaysAliveAfterHandingItsSocketOver(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { handoff.Register(nil) })

	returned := make(chan error, 1)
	go func() {
		returned <- serve(&http.Server{Handler: http.NotFoundHandler()}, ln)
	}()

	url := "http://" + ln.Addr().String() + "/"
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	for deadline := time.Now().Add(3 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if res, err := client.Get(url); err == nil {
			_ = res.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the server never answered")
		}
	}

	// What restart() does before the exec.
	_, socket := handoff.Begin(nil)
	if socket == nil {
		t.Fatal("the socket was not handed over")
	}
	defer func() { _ = socket.Close() }()

	select {
	case err := <-returned:
		t.Fatalf("serve returned (%v) after giving the socket away: the process would end before the exec", err)
	case <-time.After(400 * time.Millisecond):
	}

	// Only a signal ends it now.
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-returned:
		if err != nil {
			t.Errorf("stopping while being replaced returned %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not end on a signal")
	}
}
