package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/twostep"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/config"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

// runTwoStep is `lasthelp 2fa reset <username>`: the second step of signing in
// taken off an account, for somebody who lost the phone and the recovery codes.
// The administrator does it for anybody from the panel; this is the way back for
// the administrator's own account, which nobody above can reset. Returns the
// exit code.
func runTwoStep(args []string, out, errOut io.Writer) int {
	if len(args) != 2 || args[0] != "reset" {
		fmt.Fprintln(errOut, "usage: lasthelp 2fa reset <username>")
		return 2
	}
	cfg := config.Load()
	db, err := database.Open(cfg.DBFile)
	if err != nil {
		fmt.Fprintf(errOut, "lasthelp: could not open the database: %v\n", err)
		return 1
	}
	defer func() { _ = db.Close() }()
	return resetTwoStep(db, args[1], out, errOut)
}

func resetTwoStep(db *sql.DB, username string, out, errOut io.Writer) int {
	user, err := (account.Store{DB: db}).ByUsername(username)
	if errors.Is(err, account.ErrNotFound) {
		fmt.Fprintf(errOut, "lasthelp: there is no account called %q\n", username)
		return 1
	}
	if err != nil {
		fmt.Fprintf(errOut, "lasthelp: %v\n", err)
		return 1
	}
	tx, err := db.Begin()
	if err != nil {
		fmt.Fprintf(errOut, "lasthelp: %v\n", err)
		return 1
	}
	defer func() { _ = tx.Rollback() }()
	if err := (twostep.Store{DB: tx}).Remove(user.ID); err != nil {
		fmt.Fprintf(errOut, "lasthelp: %v\n", err)
		return 1
	}
	if err := tx.Commit(); err != nil {
		fmt.Fprintf(errOut, "lasthelp: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "Two-step sign-in is off for %s: it signs in with the password alone until it is turned on again.\n", user.Username)
	return 0
}
