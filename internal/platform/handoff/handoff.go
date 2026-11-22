// Package handoff replaces the running server without closing its port.
//
// After an update the process replaces itself with the new binary (exec). It
// used to close everything on the way: for a fraction of a second nothing
// listened on the port, so whatever arrived was refused, and the requests in
// flight were cut. A reverse proxy in front reads that as a server that failed,
// and some then stop sending to it for half a minute while it is already
// healthy: "no available server" after every update. In a portal that takes
// reports from the public, a report sent in that moment is a report lost, and
// the person who wrote it does not write it twice.
//
// Now the listening socket survives the exec. Before it, the server stops
// accepting and answers what is in flight, while a duplicate of the socket stays
// open: whatever arrives meanwhile waits in the kernel's queue. The duplicate is
// handed to the new process by its number, in Env, and the new process serves the
// queue as soon as it starts. Nobody is refused and nothing is cut short.
package handoff

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Env names the variable a replaced process finds its listening socket under.
// config reads it, and clears it: it is set by the process that replaces this
// one and by nobody else.
const Env = "LASTHELP_LISTEN_FD"

// drain is how long the requests in flight are given to finish: the new binary is waiting.
const drain = 3 * time.Second

var (
	mu sync.Mutex
	// ready readies the running server to be replaced and returns a duplicate of
	// its listening socket. Set by whoever owns the server; nil until then.
	ready func() (*os.File, error)
)

// Wrap is how a server serving on ln is handed over. The server is to serve on the
// listener returned, which counts the connections it accepts; ready, called when
// the process is about to be replaced, keeps a duplicate of the socket so the port
// never closes, stops accepting, and waits, briefly, for every connection accepted
// to be answered and closed.
//
// It does not use server.Shutdown: that closes, without a word, a connection
// accepted a moment before whose request it had not read yet, and the client sees
// the connection end with no answer (measured in Lastboard: one in a few thousand
// under load). Turning keep-alive off instead answers each one and then closes it.
//
// Serve returns net.ErrClosed once ready has run.
func Wrap(server *http.Server, ln net.Listener) (serveOn net.Listener, readyToReplace func() (*os.File, error)) {
	counted := &countingListener{Listener: ln}
	return counted, func() (*os.File, error) {
		tcp, ok := ln.(*net.TCPListener)
		if !ok {
			return nil, fmt.Errorf("the listener is not a TCP one")
		}
		socket, err := tcp.File()
		if err != nil {
			return nil, err
		}
		_ = counted.Close() // the duplicate keeps the socket open: what arrives now waits for the new process
		server.SetKeepAlivesEnabled(false)
		if !counted.waitClosed(drain) {
			slog.Warn("restart: some requests were still running and are cut")
		}
		return socket, nil
	}
}

// Register tells the updater how to hand over the running server: the function
// Wrap returned.
func Register(readyToReplace func() (*os.File, error)) {
	mu.Lock()
	defer mu.Unlock()
	ready = readyToReplace
}

// Begin readies the server to be replaced and returns the environment the new
// process is to be started with, and the socket, which has to stay open until the
// replacement has taken place (the caller keeps it alive across the exec). When
// the server cannot hand its socket over, env comes back without it and the port
// closes for a moment, as it always did: a restart that works is better than a
// restart that does not.
func Begin(env []string) (newEnv []string, socket *os.File) {
	newEnv = withoutEnv(env, Env)

	mu.Lock()
	prepare := ready
	mu.Unlock()
	if prepare == nil {
		return newEnv, nil
	}

	f, err := prepare()
	if err != nil {
		slog.Warn("restart: the server could not hand over its socket; the port closes for a moment", "error", err)
		return newEnv, nil
	}
	if err := inheritable(f); err != nil {
		slog.Warn("restart: the socket cannot be passed on; the port closes for a moment", "error", err)
		_ = f.Close()
		return newEnv, nil
	}
	return append(newEnv, fmt.Sprintf("%s=%d", Env, f.Fd())), f
}

// Inherit is the listening socket a replaced process handed over, or nil when there
// is none or it cannot be used (then the caller listens as it always did). raw is
// what Env held; addr is the address the server was asked to listen on. A socket
// that is not on that port is not used.
func Inherit(raw, addr string) net.Listener {
	if raw == "" {
		return nil
	}
	fd, err := strconv.Atoi(raw)
	if err != nil || fd < 3 {
		slog.Warn("restart: ignoring a listening socket that is not one", "value", raw)
		return nil
	}
	f := os.NewFile(uintptr(fd), "inherited-listener")
	if f == nil {
		return nil
	}
	ln, err := net.FileListener(f) // a duplicate of its own; the inherited number is closed below
	_ = f.Close()
	if err != nil {
		slog.Warn("restart: the inherited socket cannot be listened on", "error", err)
		return nil
	}
	if !sameAddress(ln.Addr(), addr) {
		slog.Warn("restart: the inherited socket is for another address; listening anew", "socket", ln.Addr().String(), "wanted", addr)
		_ = ln.Close()
		return nil
	}
	return ln
}

// sameAddress says whether a socket is the one an address (":8080", "127.0.0.1:8080") asks for.
func sameAddress(have net.Addr, want string) bool {
	tcp, ok := have.(*net.TCPAddr)
	if !ok {
		return false
	}
	host, port, err := net.SplitHostPort(want)
	if err != nil || strconv.Itoa(tcp.Port) != port {
		return false
	}
	if host == "" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	got, ok := netip.AddrFromSlice(tcp.IP)
	return ok && got.Unmap() == ip.Unmap()
}

// countingListener counts the connections it accepted that are still open.
type countingListener struct {
	net.Listener
	open atomic.Int64
}

func (l *countingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.open.Add(1)
	return &countedConn{Conn: c, done: func() { l.open.Add(-1) }}, nil
}

// waitClosed reports whether every connection accepted was closed within d.
func (l *countingListener) waitClosed(d time.Duration) bool {
	for deadline := time.Now().Add(d); l.open.Load() > 0; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			return false
		}
	}
	return true
}

// countedConn tells its listener when it closes, once, however many times Close is called.
type countedConn struct {
	net.Conn
	once sync.Once
	done func()
}

func (c *countedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(c.done)
	return err
}

// withoutEnv is env without the variable named.
func withoutEnv(env []string, name string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, name+"=") {
			out = append(out, kv)
		}
	}
	return out
}
