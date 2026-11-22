package handoff

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"
)

// The helper is this test binary run again as a server: generation 1 serves for a
// moment and then replaces itself exactly as an update does (Begin, then exec),
// becoming generation 2.
const (
	helperEnv     = "LH_RESTART_HELPER"
	helperGenEnv  = "LH_RESTART_GEN"
	helperPortEnv = "LH_RESTART_PORT"
	// helperBareEnv makes the helper restart the way it did before: no handoff,
	// the port closes for a moment.
	helperBareEnv = "LH_RESTART_BARE"
)

func restartHelper() {
	port := os.Getenv(helperPortEnv)
	gen := os.Getenv(helperGenEnv)
	addr := "127.0.0.1:" + port

	ln := Inherit(os.Getenv(Env), addr)
	_ = os.Unsetenv(Env) // what config does
	if ln == nil {
		var err error
		if ln, err = net.Listen("tcp", addr); err != nil {
			fmt.Fprintln(os.Stderr, "helper:", err)
			os.Exit(2)
		}
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, gen)
	})}
	if os.Getenv(helperBareEnv) == "" {
		serveOn, ready := Wrap(server, ln)
		go func() { _ = server.Serve(serveOn) }()
		Register(ready)
	} else {
		go func() { _ = server.Serve(ln) }()
	}
	if gen == "1" {
		time.Sleep(700 * time.Millisecond)
		_ = os.Setenv(helperGenEnv, "2")
		exe, err := os.Executable()
		if err != nil {
			os.Exit(2)
		}
		env, socket := Begin(os.Environ())
		err = syscall.Exec(exe, os.Args, env)
		runtime.KeepAlive(socket)
		fmt.Fprintln(os.Stderr, "helper: exec failed:", err)
		os.Exit(1)
	}
	select {} // generation 2 serves until the test stops it
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
}

// restartUnderLoad starts the helper, opens new connections without pause while it
// replaces itself, and tells how many were refused or cut, and which generations
// answered.
func restartUnderLoad(t *testing.T, bare bool) (failed []string, gens map[string]int) {
	t.Helper()
	port := freePort(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestARestartNeverRefusesAConnection$", "-test.count=1")
	cmd.Env = append(os.Environ(), helperEnv+"=1", helperGenEnv+"=1", helperPortEnv+"="+port)
	if bare {
		cmd.Env = append(cmd.Env, helperBareEnv+"=1")
	}
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	url := "http://127.0.0.1:" + port + "/"
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if res, err := client.Get(url); err == nil {
			_ = res.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the helper never answered: %s", logs.String())
		}
	}

	var mu sync.Mutex
	gens = map[string]int{}
	var wg sync.WaitGroup
	stop := time.Now().Add(2500 * time.Millisecond)
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(stop) {
				res, err := client.Get(url)
				mu.Lock()
				if err != nil {
					failed = append(failed, time.Now().Format("15:04:05.000")+" "+err.Error())
				} else {
					body, _ := io.ReadAll(res.Body)
					_ = res.Body.Close()
					gens[string(body)]++
				}
				mu.Unlock()
				time.Sleep(2 * time.Millisecond)
			}
		}()
	}
	wg.Wait()
	if gens["2"] == 0 {
		t.Fatalf("the helper never came back as the new generation (%v): %s", gens, logs.String())
	}
	return failed, gens
}

// After an update the server replaces itself. A proxy in front must see nothing
// fail meanwhile: every connection opened while it happens is answered, by the old
// process or by the new one, and none is refused or cut. A refused one is what
// makes a proxy stop sending to a server that is already healthy.
func TestARestartNeverRefusesAConnection(t *testing.T) {
	if os.Getenv(helperEnv) != "" {
		restartHelper()
		return
	}
	if testing.Short() {
		t.Skip("starts a server process")
	}
	failed, gens := restartUnderLoad(t, false)
	if len(failed) != 0 {
		t.Errorf("%d connection(s) failed during the restart (answered: %v): %v", len(failed), gens, failed)
	}
	if gens["1"] == 0 {
		t.Errorf("nothing was answered before the restart: the load did not cover it (%v)", gens)
	}
}

// The control: the same load against a restart that hands nothing over does lose
// connections. Without this, a clean run above could mean the load never reached
// the moment that matters.
func TestWithoutTheHandoffTheSameLoadDoesLoseConnections(t *testing.T) {
	if os.Getenv(helperEnv) != "" {
		return // the helper is run by the test above
	}
	if testing.Short() {
		t.Skip("starts a server process")
	}
	failed, gens := restartUnderLoad(t, true)
	if len(failed) == 0 {
		t.Fatalf("a restart with no handoff lost nothing (answered: %v): the load above proves nothing", gens)
	}
}

// A process started after a replacement does not take a socket that is not one,
// nor one on another address.
//
// Every number handed over here is a raw descriptor that belongs to nobody else,
// as it does in a process that inherited it: Inherit closes it. One still held by
// an *os.File would be closed a second time by that file's finalizer, by then
// perhaps another test's database.
func TestOnlyAUsableSocketIsTakenFromTheProcessReplaced(t *testing.T) {
	for _, value := range []string{"", "x", "1", "-4"} {
		if ln := Inherit(value, ":8080"); ln != nil {
			_ = ln.Close()
			t.Errorf("%q was taken as a socket", value)
		}
	}
	// A number below three is a standard stream, never a socket handed over, and
	// discarding it must not close it: closing what it is told is not a socket
	// would take the process's own output with it.
	for fd := 0; fd <= 2; fd++ {
		_ = Inherit(strconv.Itoa(fd), ":8080")
		var st syscall.Stat_t
		if err := syscall.Fstat(fd, &st); err != nil {
			t.Fatalf("taking %d for a socket closed it: %v", fd, err)
		}
	}

	notASocket, err := syscall.Open(os.DevNull, syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ln := Inherit(strconv.Itoa(notASocket), ":8080"); ln != nil {
		_ = ln.Close()
		t.Error("a file that is not a socket was taken")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	handOver := func() string {
		raw, err := ln.(*net.TCPListener).SyscallConn()
		if err != nil {
			t.Fatal(err)
		}
		var fd int
		var dupErr error
		if err := raw.Control(func(s uintptr) { fd, dupErr = syscall.Dup(int(s)) }); err != nil || dupErr != nil {
			t.Fatal(err, dupErr)
		}
		syscall.CloseOnExec(fd)
		return strconv.Itoa(fd)
	}

	if other := Inherit(handOver(), "127.0.0.1:1"); other != nil {
		_ = other.Close()
		t.Error("a socket on another port was taken")
	}
	if other := Inherit(handOver(), "127.0.0.2:"+port); other != nil {
		_ = other.Close()
		t.Error("a socket on another address was taken")
	}
	got := Inherit(handOver(), "127.0.0.1:"+port)
	if got == nil {
		t.Fatal("the socket on the right address was not taken")
	}
	_ = got.Close()
	got = Inherit(handOver(), ":"+port)
	if got == nil {
		t.Fatal("the socket was not taken when the address names only the port")
	}
	_ = got.Close()
}

// With nothing registered (a test, or a platform that cannot do it) the new
// process gets the environment it had, minus any socket number left from an
// earlier replacement, and the restart goes on as it always did.
func TestWithNothingRegisteredTheEnvironmentCarriesNoSocket(t *testing.T) {
	Register(nil)
	env, socket := Begin([]string{"A=1", Env + "=7", "B=2"})
	if socket != nil {
		t.Error("a socket was returned with nothing to hand over")
	}
	for _, kv := range env {
		if kv == Env+"=7" {
			t.Error("a stale socket number was passed on to the next process")
		}
	}
	if len(env) != 2 {
		t.Errorf("env = %v", env)
	}
}
