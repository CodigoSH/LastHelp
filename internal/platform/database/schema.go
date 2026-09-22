package database

import "database/sql"

// migrations is the whole history of this schema, append-only.
//
// Nothing here is ever renumbered, reworded into a different meaning, or
// deleted. A database in the wild stores the number of the last step it ran, so
// changing what a number means changes what that database believes about
// itself.
var migrations = []migration{
	{
		version: 1,
		name:    "initial schema",
		stmts: []string{
			// --- The structure: an organisation is a forge connection ------
			//
			// The credential lives here because the forge will not let it live
			// anywhere else: a GitHub fine-grained token is bound to a single
			// owner and cannot span two organisations, and GitLab issues group
			// tokens per group. Two organisations are two connections whether
			// we like it or not.
			`CREATE TABLE organization (
				id                 INTEGER PRIMARY KEY,
				slug               TEXT    NOT NULL UNIQUE,
				name               TEXT    NOT NULL,
				forge_kind         TEXT    NOT NULL,
				base_url           TEXT    NOT NULL,
				forge_owner        TEXT    NOT NULL,
				token_enc          BLOB,
				webhook_secret_enc BLOB,
				created_at         INTEGER NOT NULL
			)`,

			// Only the repositories the operator wants feedback for. A project
			// spread over twenty repositories registers the one it wants
			// reports in, rather than making a reporter guess.
			`CREATE TABLE repository (
				id              INTEGER PRIMARY KEY,
				org_id          INTEGER NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
				slug            TEXT    NOT NULL,
				name            TEXT    NOT NULL,
				forge_repo      TEXT    NOT NULL,
				accepts_bugs    INTEGER NOT NULL DEFAULT 1,
				accepts_ideas   INTEGER NOT NULL DEFAULT 1,
				moderation_mode TEXT    NOT NULL DEFAULT 'all',
				notify_config   TEXT    NOT NULL DEFAULT '{}',
				created_at      INTEGER NOT NULL,
				UNIQUE (org_id, slug)
			)`,

			// --- The team --------------------------------------------------
			//
			// Named app_user because `user` is a keyword in enough engines that
			// somebody porting a query will trip over it.
			`CREATE TABLE app_user (
				id               INTEGER PRIMARY KEY,
				username         TEXT    NOT NULL UNIQUE,
				password_hash    TEXT    NOT NULL,
				display_name     TEXT    NOT NULL DEFAULT '',
				is_administrator INTEGER NOT NULL DEFAULT 0,
				disabled         INTEGER NOT NULL DEFAULT 0,
				avatar           TEXT    NOT NULL DEFAULT '',
				dashboard        TEXT    NOT NULL DEFAULT '',
				created_at       INTEGER NOT NULL
			)`,

			// The administrator is a flag on the account, not a row here: it is
			// a property of the server rather than of any one organisation.
			// Everything else is scoped, and a person who helps with four
			// repositories has four rows.
			`CREATE TABLE membership (
				id         INTEGER PRIMARY KEY,
				user_id    INTEGER NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
				scope_kind TEXT    NOT NULL,
				scope_id   INTEGER NOT NULL,
				role       TEXT    NOT NULL,
				created_at INTEGER NOT NULL,
				UNIQUE (user_id, scope_kind, scope_id)
			)`,

			`CREATE TABLE session (
				token_hash TEXT    PRIMARY KEY,
				user_id    INTEGER NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
				created_at INTEGER NOT NULL,
				expires_at INTEGER NOT NULL,
				last_seen  INTEGER NOT NULL
			)`,

			// --- What the public writes -------------------------------------
			//
			// A bug and an idea share this table because they share almost
			// everything: moderation, comments, the link to an issue, search.
			// They differ in two things — only ideas are voted, and only bugs
			// carry form fields — and both of those are enforced in the domain,
			// where the reason can be written down, rather than by splitting
			// the table and duplicating the other 90%.
			//
			// follow_token_hash is a hash, like a session: a leaked database
			// must not hand anybody the ability to reply as the reporter.
			`CREATE TABLE item (
				id                INTEGER PRIMARY KEY,
				public_id         TEXT    NOT NULL UNIQUE,
				repo_id           INTEGER NOT NULL REFERENCES repository(id) ON DELETE CASCADE,
				kind              TEXT    NOT NULL,
				title             TEXT    NOT NULL,
				body              TEXT    NOT NULL,
				status            TEXT    NOT NULL,
				moderation_state  TEXT    NOT NULL DEFAULT 'pending',
				vote_count        INTEGER NOT NULL DEFAULT 0,
				body_hash         TEXT    NOT NULL DEFAULT '',
				author_email_enc  BLOB,
				follow_token_hash TEXT    NOT NULL,
				merged_into       INTEGER REFERENCES item(id) ON DELETE SET NULL,
				created_at        INTEGER NOT NULL,
				published_at      INTEGER,
				closed_at         INTEGER
			)`,

			`CREATE INDEX item_by_repo ON item (repo_id, moderation_state, status)`,
			`CREATE INDEX item_by_public_id ON item (public_id)`,
			// The flood of copy-paste spam is the same text over and over, and
			// this is what makes noticing it a lookup rather than a read.
			`CREATE INDEX item_by_body_hash ON item (body_hash)`,

			// The answers are stored with the label they were asked under, not
			// only the key. Renaming or deleting a field later must not rewrite
			// what somebody already sent.
			`CREATE TABLE item_field (
				id       INTEGER PRIMARY KEY,
				item_id  INTEGER NOT NULL REFERENCES item(id) ON DELETE CASCADE,
				key      TEXT    NOT NULL,
				label    TEXT    NOT NULL,
				value    TEXT    NOT NULL,
				position INTEGER NOT NULL DEFAULT 0
			)`,
			`CREATE INDEX item_field_by_item ON item_field (item_id, position)`,

			// Four types and no more. A form with fifteen field types is a
			// questionnaire nobody fills in.
			`CREATE TABLE field_def (
				id       INTEGER PRIMARY KEY,
				repo_id  INTEGER NOT NULL REFERENCES repository(id) ON DELETE CASCADE,
				key      TEXT    NOT NULL,
				label    TEXT    NOT NULL,
				type     TEXT    NOT NULL,
				required INTEGER NOT NULL DEFAULT 0,
				options  TEXT    NOT NULL DEFAULT '',
				position INTEGER NOT NULL DEFAULT 0,
				UNIQUE (repo_id, key)
			)`,

			// The voter identity is a random id from a signed cookie, never the
			// address: behind a household router or a mobile carrier the
			// address is shared, and keying votes on it would let one neighbour
			// spend everybody else's vote.
			`CREATE TABLE vote (
				item_id    INTEGER NOT NULL REFERENCES item(id) ON DELETE CASCADE,
				voter_id   TEXT    NOT NULL,
				created_at INTEGER NOT NULL,
				PRIMARY KEY (item_id, voter_id)
			)`,

			`CREATE TABLE comment (
				id               INTEGER PRIMARY KEY,
				item_id          INTEGER NOT NULL REFERENCES item(id) ON DELETE CASCADE,
				author_kind      TEXT    NOT NULL,
				author_ref       TEXT    NOT NULL DEFAULT '',
				body             TEXT    NOT NULL,
				moderation_state TEXT    NOT NULL DEFAULT 'pending',
				created_at       INTEGER NOT NULL
			)`,
			`CREATE INDEX comment_by_item ON comment (item_id, created_at)`,

			// echo_fingerprint is what stops the loop. Every write we send
			// leaves its mark here, and the webhook that comes back carrying it
			// is our own change arriving as news.
			`CREATE TABLE issue_link (
				item_id          INTEGER PRIMARY KEY REFERENCES item(id) ON DELETE CASCADE,
				repo_id          INTEGER NOT NULL REFERENCES repository(id) ON DELETE CASCADE,
				forge_number     INTEGER NOT NULL,
				forge_url        TEXT    NOT NULL,
				last_state       TEXT    NOT NULL DEFAULT '',
				last_synced_at   INTEGER,
				echo_fingerprint TEXT    NOT NULL DEFAULT ''
			)`,
			`CREATE INDEX issue_link_by_number ON issue_link (repo_id, forge_number)`,

			// --- The machinery ----------------------------------------------
			//
			// No call to a forge is made from a handler. Forges rate-limit and
			// forges go down, and a request that fails because somebody else's
			// server is having a bad afternoon is a report that got lost.
			`CREATE TABLE outbox (
				id              INTEGER PRIMARY KEY,
				kind            TEXT    NOT NULL,
				payload         TEXT    NOT NULL,
				attempts        INTEGER NOT NULL DEFAULT 0,
				next_attempt_at INTEGER NOT NULL,
				last_error      TEXT    NOT NULL DEFAULT '',
				created_at      INTEGER NOT NULL
			)`,
			`CREATE INDEX outbox_due ON outbox (next_attempt_at)`,

			// Deliveries are replayed, by the forge and by anybody who captured
			// one. Bounded on purpose: an unbounded table keyed on something a
			// caller controls is a way to fill the disk.
			`CREATE TABLE webhook_delivery (
				org_id      INTEGER NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
				delivery_id TEXT    NOT NULL,
				received_at INTEGER NOT NULL,
				PRIMARY KEY (org_id, delivery_id)
			)`,
			`CREATE INDEX webhook_delivery_age ON webhook_delivery (received_at)`,

			// Single-use, so a token that was spent cannot be spent again.
			`CREATE TABLE form_token (
				token_hash TEXT    PRIMARY KEY,
				used_at    INTEGER NOT NULL,
				expires_at INTEGER NOT NULL
			)`,
			`CREATE INDEX form_token_expiry ON form_token (expires_at)`,

			`CREATE TABLE settings (
				key   TEXT PRIMARY KEY,
				value TEXT NOT NULL
			)`,

			// --- Search ------------------------------------------------------
			//
			// One index serves both jobs: showing a reporter the three things
			// that look like what they are about to send, and the public
			// search. An external-content table keeps the text in `item` and
			// the triggers keep the index honest — the alternative is a second
			// copy of every title and body that drifts the first time somebody
			// edits one.
			`CREATE VIRTUAL TABLE item_fts USING fts5(
				title, body, content='item', content_rowid='id'
			)`,
			`CREATE TRIGGER item_fts_after_insert AFTER INSERT ON item BEGIN
				INSERT INTO item_fts (rowid, title, body) VALUES (new.id, new.title, new.body);
			END`,
			`CREATE TRIGGER item_fts_after_delete AFTER DELETE ON item BEGIN
				INSERT INTO item_fts (item_fts, rowid, title, body) VALUES ('delete', old.id, old.title, old.body);
			END`,
			`CREATE TRIGGER item_fts_after_update AFTER UPDATE ON item BEGIN
				INSERT INTO item_fts (item_fts, rowid, title, body) VALUES ('delete', old.id, old.title, old.body);
				INSERT INTO item_fts (rowid, title, body) VALUES (new.id, new.title, new.body);
			END`,
		},
	},
	{
		version: 2,
		name:    "opaque webhook address per organisation",
		// The address a forge delivers to must not be guessable from anything
		// public, and it must identify the connection on its own — because the
		// receiver is chosen from this record and never from the request's
		// headers. Forgejo sends GitHub's headers as well as its own, so a
		// server that decided what a delivery was by looking at them would let
		// the caller choose which verification it faced.
		//
		// Added by ALTER rather than only in the CREATE above, because the
		// CREATE only ever runs on a fresh install and every database that
		// already exists gets this column from here.
		stmts: []string{
			`ALTER TABLE organization ADD COLUMN hook_id TEXT NOT NULL DEFAULT ''`,
			// After the ALTER, never before: an index on a column that does not
			// exist yet fails with "no such column" on every existing database
			// and on none of the new ones.
			`CREATE INDEX organization_by_hook ON organization (hook_id)`,
		},
	},
	{
		version: 3,
		name:    "accounts are named by a username, not an address",
		// An address was never what this column held: it identified the account
		// and nothing was ever sent to it. Asking somebody setting up their own
		// server for an email address they will never receive anything at is
		// asking for information to store and not use.
		//
		// RENAME COLUMN rather than a rebuild, and skipped when the column is
		// already called username — which is every database created by the
		// CREATE above, and this migration runs on those too.
		fn: renameEmailToUsername,
	},
	{
		version: 4,
		name:    "an account can carry a picture",
		// Stored as a data URL in the row rather than as a file on disk: it is
		// one small image per account, it has to survive a backup, and a backup
		// that is one file only stays one file if nothing lives outside it.
		// The size bound is enforced where it is uploaded.
		stmts: []string{},
		fn: func(tx *sql.Tx) error {
			return addColumn(tx, "app_user", "avatar", "TEXT NOT NULL DEFAULT ''")
		},
	},
	{
		version: 5,
		name:    "an account keeps its own dashboard",
		// The order and the visibility of the overview's cards, as JSON. Per
		// account rather than per browser: somebody who arranges their dashboard
		// on a desktop and opens it on a laptop has not arranged anything if it
		// lives in localStorage.
		fn: func(tx *sql.Tx) error {
			return addColumn(tx, "app_user", "dashboard", "TEXT NOT NULL DEFAULT ''")
		},
	},
	{
		version: 6,
		name:    "an account keeps its own language",
		// Per account rather than per browser, for the same reason as the
		// dashboard: a language chosen on a desktop and not on the laptop is a
		// setting that did not take.
		//
		// Empty means "not chosen", which is not the same as English: an empty
		// value follows the browser, and English is what everything falls back
		// to when the browser asks for a language this does not have.
		fn: func(tx *sql.Tx) error {
			return addColumn(tx, "app_user", "language", "TEXT NOT NULL DEFAULT ''")
		},
	},
	{
		version: 7,
		name:    "secrets are stored as they are given",
		// The three sealed columns become plain ones beside them.
		//
		// What was sealed — a forge token, a webhook secret, the address of
		// somebody who asked to be told when their report moved — is the
		// operator's own data on the operator's own machine, and the key that
		// opened it sat in the same directory. Against the one thing encryption
		// at rest can cover, a copy of the file leaving the machine, a key that
		// travels beside the data protects nothing. The boundary is the backup,
		// and that is where the password belongs.
		//
		// The old columns are left exactly where they are. A migration that
		// deletes one re-breaks every database old enough to still need it, and
		// the rule holds even when the column is one this code no longer reads.
		fn: func(tx *sql.Tx) error {
			for _, c := range []struct{ table, column string }{
				{"organization", "token"},
				{"organization", "webhook_secret"},
				{"item", "author_email"},
			} {
				if err := addColumn(tx, c.table, c.column, "TEXT NOT NULL DEFAULT ''"); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		version: 8,
		name:    "a picture an operator puts on the front page",
		// Bytes, not a data URL in the layout.
		//
		// The layout is one JSON value read on every render of the public page
		// and bounded at a few thousand characters; a photograph encoded into
		// it would blow that bound and be re-read by every visitor. The layout
		// keeps a reference, the bytes live here, and they are served with a
		// long cache because the name of a row never changes what is in it.
		fn: func(tx *sql.Tx) error {
			_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS landing_image (
				id          TEXT PRIMARY KEY,
				bytes       BLOB NOT NULL,
				content_type TEXT NOT NULL,
				created_at  TEXT NOT NULL
			)`)
			return err
		},
	},
}

// renameEmailToUsername is a no-op on any database that already has the column.
func renameEmailToUsername(tx *sql.Tx) error {
	has, err := hasColumn(tx, "app_user", "username")
	if err != nil || has {
		return err
	}
	_, err = tx.Exec(`ALTER TABLE app_user RENAME COLUMN email TO username`)
	return err
}
