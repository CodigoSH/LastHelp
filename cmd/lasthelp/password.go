package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/session"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/config"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

// minPasswordLength is the floor the panel enforces, held here too: a password
// set from the terminal must be one the sign-in form would have accepted.
const minPasswordLength = 10

// secretReader asks the person at the keyboard for something that must not be
// shown or kept: a terminal in the real command, a script in the tests.
type secretReader interface {
	Terminal() bool
	Ask(prompt string) ([]byte, error)
}

// console is the terminal the command was started from.
type console struct{ fd int }

func (c console) Terminal() bool { return term.IsTerminal(c.fd) }

// Ask writes the prompt and reads what is typed without echoing it.
//
// term.ReadPassword puts the echo back when it returns, but an interrupt ends
// the process before it can — and a shell left not showing what is typed looks
// broken. So an interrupt puts the terminal back first.
func (c console) Ask(prompt string) ([]byte, error) {
	fmt.Fprint(os.Stderr, prompt)
	state, err := term.GetState(c.fd)
	if err != nil {
		return nil, err
	}
	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	defer func() { signal.Stop(interrupted); close(done) }()
	go func() {
		select {
		case <-interrupted:
			_ = term.Restore(c.fd, state)
			fmt.Fprintln(os.Stderr)
			os.Exit(130)
		case <-done:
		}
	}()
	secret, err := term.ReadPassword(c.fd)
	fmt.Fprintln(os.Stderr)
	return secret, err
}

// runPassword is `lasthelp password reset <username>`: the way back for an
// account whose password is forgotten — above all the administrator's, which
// nobody else can reset, on an instance that has no email to send a link to.
// The password is asked for on the terminal, twice, so it never reaches the
// shell's history or the process list. Returns the exit code.
func runPassword(args []string, in secretReader, out, errOut io.Writer) int {
	if len(args) != 2 || args[0] != "reset" {
		fmt.Fprintln(errOut, "usage: lasthelp password reset <username>")
		return 2
	}
	if !in.Terminal() {
		fmt.Fprintln(errOut, "lasthelp: this asks for the new password and needs a terminal; in Docker, run it with docker exec -it")
		return 1
	}

	cfg := config.Load()
	db, err := database.Open(cfg.DBFile)
	if err != nil {
		fmt.Fprintf(errOut, "lasthelp: could not open the database: %v\n", err)
		return 1
	}
	defer func() { _ = db.Close() }()

	return resetPassword(db, args[1], in, out, errOut)
}

func resetPassword(db database.Execer, username string, in secretReader, out, errOut io.Writer) int {
	accounts := account.Store{DB: db}

	// Before asking for a password nobody will use.
	user, err := accounts.ByUsername(username)
	if errors.Is(err, account.ErrNotFound) {
		fmt.Fprintf(errOut, "lasthelp: there is no account called %q\n", username)
		return 1
	}
	if err != nil {
		fmt.Fprintf(errOut, "lasthelp: %v\n", err)
		return 1
	}

	first, err := in.Ask(fmt.Sprintf("New password for %s: ", user.Username))
	if err != nil {
		fmt.Fprintf(errOut, "lasthelp: reading the password: %v\n", err)
		return 1
	}
	defer clear(first)
	if len(first) < minPasswordLength {
		fmt.Fprintf(errOut, "lasthelp: a password needs at least %d characters; nothing was changed\n", minPasswordLength)
		return 1
	}
	second, err := in.Ask("Type it again: ")
	if err != nil {
		fmt.Fprintf(errOut, "lasthelp: reading the password: %v\n", err)
		return 1
	}
	defer clear(second)
	if !bytes.Equal(first, second) {
		fmt.Fprintln(errOut, "lasthelp: the two passwords are not the same; nothing was changed")
		return 1
	}

	if err := accounts.ResetPassword(user.ID, string(first)); err != nil {
		fmt.Fprintf(errOut, "lasthelp: %v\n", err)
		return 1
	}
	// What changing the password does from the panel: whoever had the old one is
	// signed out. A reset that left their sessions open would not take the
	// account back from them.
	if err := (session.Store{DB: db}).DeleteAllFor(user.ID); err != nil {
		fmt.Fprintf(errOut, "lasthelp: the password was changed but its sessions could not be closed: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "The password of %s is changed, and every session that account had is closed.\n", user.Username)
	return 0
}
