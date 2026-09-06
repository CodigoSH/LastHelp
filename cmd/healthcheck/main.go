// Command healthcheck asks the server next to it whether it is alive.
//
// It exists as a separate binary because the container image ships no shell and
// no curl. The only address it ever uses is loopback: it is not a monitoring
// tool, it answers one question about the process it shares a container with.
package main

import (
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/config"
)

func main() {
	// The address comes from the same place the server's does. Reading the
	// environment here instead would be a second reader of the same setting,
	// and the two drift: this binary used to look only at LASTHELP_PORT, so an
	// operator who set LASTHELP_ADDR got a healthcheck probing the wrong port
	// and a container that restarted itself forever.
	cfg := config.Load()

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + loopback(cfg.Addr) + "/healthz")
	if err != nil {
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
}

// loopback turns the listening address into one to dial. A server listening on
// every interface is reached on 127.0.0.1; one bound to a single address is
// reached there.
func loopback(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return net.JoinHostPort("127.0.0.1", strings.TrimPrefix(addr, ":"))
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
