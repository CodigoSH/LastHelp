package web

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/secretbox"
)

// unsealOnce moves what an earlier version encrypted into the plain columns
// beside it, and is the last thing in this product that knows how to open a
// sealed value.
//
// The schema migration adds the columns; this fills them. It has to be here
// rather than in a migration because it needs the key, and the key is a file
// this layer is given — the database package is handed an *sql.DB and nothing
// else, which is the rule that keeps it from growing opinions about the rest of
// the program.
//
// It runs at startup, it is idempotent — a row whose plain column is already
// set is left alone — and it stops trying once there is nothing sealed left.
//
// When the key file is gone, the sealed bytes cannot be opened by anybody,
// including us. That is not a failure to report at startup: it is a fact to say
// once, clearly, so an operator knows to paste the token again rather than
// wondering why the bridge stopped.
func unsealOnce(db *sql.DB, keyFile string) error {
	sealed, err := countSealed(db)
	if err != nil {
		return err
	}
	if sealed == 0 {
		return nil
	}

	box, err := secretbox.OpenExisting(keyFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, secretbox.ErrNoKey) {
			slog.Warn("there are encrypted values from an earlier version and no key to open them",
				"rows", sealed,
				"what", "the forge token and webhook secret have to be pasted again in the panel")
			return nil
		}
		return fmt.Errorf("opening the key to migrate encrypted values: %w", err)
	}

	moved, failed := 0, 0
	move := func(table, sealedCol, plainCol, idCol string) error {
		rows, err := db.Query(fmt.Sprintf(
			"SELECT %s, %s FROM %s WHERE %s IS NOT NULL AND length(%s) > 0 AND %s = ''",
			idCol, sealedCol, table, sealedCol, sealedCol, plainCol))
		if err != nil {
			return err
		}
		type pair struct {
			id    int64
			bytes []byte
		}
		var pending []pair
		for rows.Next() {
			var p pair
			if err := rows.Scan(&p.id, &p.bytes); err != nil {
				_ = rows.Close()
				return err
			}
			pending = append(pending, p)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		_ = rows.Close()

		for _, p := range pending {
			plain, err := box.Open(p.bytes)
			if err != nil {
				// One unreadable row does not stop the others: a value sealed
				// with a key that has since been replaced is gone, and gone is
				// something the operator can fix by pasting it again.
				failed++
				continue
			}
			if _, err := db.Exec(
				fmt.Sprintf("UPDATE %s SET %s = ? WHERE %s = ?", table, plainCol, idCol),
				plain, p.id); err != nil {
				return err
			}
			moved++
		}
		return nil
	}

	for _, c := range []struct{ table, sealedCol, plainCol, idCol string }{
		{"organization", "token_enc", "token", "id"},
		{"organization", "webhook_secret_enc", "webhook_secret", "id"},
		{"item", "author_email_enc", "author_email", "id"},
	} {
		if err := move(c.table, c.sealedCol, c.plainCol, c.idCol); err != nil {
			return fmt.Errorf("moving %s.%s: %w", c.table, c.sealedCol, err)
		}
	}

	if moved > 0 {
		slog.Info("encrypted values from an earlier version moved into plain columns",
			"moved", moved,
			"what", "the key file beside the database is read by nothing now and can be deleted")
	}
	if failed > 0 {
		// This one repeats on every start, and that is the honest thing: the
		// bytes stay in place in case the key turns up on an old server, so
		// the row keeps counting as pending until somebody deals with it.
		slog.Warn("some encrypted values from an earlier version cannot be opened with this key",
			"values", failed,
			"what", "enter them again in the panel; what is already entered is not touched")
	}
	return nil
}

// countSealed is the cheap question asked before the expensive one: on every
// start after the first there is nothing sealed, and this answers in one scan
// without opening a key file that no longer has to exist.
func countSealed(db *sql.DB) (int, error) {
	var n int
	err := db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM organization
			  WHERE (token_enc IS NOT NULL AND length(token_enc) > 0 AND token = '')
			     OR (webhook_secret_enc IS NOT NULL AND length(webhook_secret_enc) > 0 AND webhook_secret = ''))
			+
			(SELECT COUNT(*) FROM item
			  WHERE author_email_enc IS NOT NULL AND length(author_email_enc) > 0 AND author_email = '')
	`).Scan(&n)
	return n, err
}
