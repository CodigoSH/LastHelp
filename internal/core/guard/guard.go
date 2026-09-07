// Package guard is what stands between a public form and the moderation queue.
//
// What it protects is not the database: nothing is published without approval,
// so spam never reaches a public page. What it protects is **the operator's
// attention**. The failure it exists to prevent is a queue so full of rubbish
// that the person stops opening it, and a portal nobody moderates reads worse
// from outside than no portal at all.
//
// Nothing here calls a third party. No captcha service, no reputation API, no
// model: the whole point of the product is that what people write reaches one
// database and nowhere else.
package guard

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

var (
	// ErrNoToken means the form was never fetched: this is a POST straight at
	// the endpoint, which is what most automated submission is.
	ErrNoToken = errors.New("no form token")

	// ErrTooFast means the form was filled in faster than a person types.
	ErrTooFast = errors.New("submitted too quickly")

	// ErrStale means the token is old enough to be a replay.
	ErrStale = errors.New("the form went stale")

	// ErrUsed means this token already submitted something.
	ErrUsed = errors.New("that form was already used")

	// ErrHoneypot means a field no person can see was filled in.
	ErrHoneypot = errors.New("honeypot")

	// ErrClosed means the circuit breaker tripped: this board is taking more
	// than a human could read, so it stops taking anything for a while.
	ErrClosed = errors.New("this board is not taking submissions right now")
)

// Timing. A form filled in under two seconds was not typed; one submitted two
// hours later is a token somebody kept.
const (
	MinFillTime = 2 * time.Second
	MaxFillTime = 2 * time.Hour
)

// HourlyLimit is how many submissions a board takes in an hour before the
// breaker trips. Chosen to be far above what a real board sees and far below
// what a flood produces — the number that matters is that somebody can still
// read the queue afterwards.
const HourlyLimit = 60

// Guard holds what the checks need.
type Guard struct {
	DB database.Execer

	// Key signs form tokens. The server's own, generated once.
	Key []byte
}

// --- the form token ---------------------------------------------------------

// IssueToken mints the token the form carries.
//
// It binds three things: which board the form was fetched for, when, and which
// address bucket asked. A token minted for one board cannot submit to another,
// and one minted an hour ago in a different country cannot be replayed here.
func (g Guard) IssueToken(repoID int64, bucket string, now time.Time) string {
	payload := fmt.Sprintf("%d.%d.%s", repoID, now.Unix(), shortHash(bucket))
	return payload + "." + base64.RawURLEncoding.EncodeToString(g.sign(payload))
}

// CheckToken accepts a submission's token, or says why not.
//
// On success the token is spent: stored so the same one cannot be used twice.
// That is what turns "fetch the form once, submit a thousand times" into "fetch
// the form a thousand times", which costs the sender something.
func (g Guard) CheckToken(token string, repoID int64, bucket string, now time.Time) error {
	if token == "" {
		return ErrNoToken
	}

	parts := strings.Split(token, ".")
	if len(parts) != 4 {
		return ErrNoToken
	}
	payload := strings.Join(parts[:3], ".")

	given, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || !hmac.Equal(given, g.sign(payload)) {
		return ErrNoToken
	}

	tokenRepo, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || tokenRepo != repoID {
		return ErrNoToken
	}
	if parts[2] != shortHash(bucket) {
		// Minted for a different part of the internet. A mobile connection can
		// legitimately change address mid-form, which is why this is a refusal
		// with a fresh form rather than a ban.
		return ErrStale
	}

	issued, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return ErrNoToken
	}
	age := now.Sub(time.Unix(issued, 0))
	switch {
	case age < MinFillTime:
		return ErrTooFast
	case age > MaxFillTime:
		return ErrStale
	}

	return g.spend(token, now)
}

// spend records the token, and fails if it was already there. The uniqueness is
// the primary key doing the work, so two requests racing cannot both win.
func (g Guard) spend(token string, now time.Time) error {
	res, err := g.DB.Exec(`INSERT INTO form_token (token_hash, used_at, expires_at)
		VALUES (?, ?, ?) ON CONFLICT (token_hash) DO NOTHING`,
		hashToken(token), now.Unix(), now.Add(MaxFillTime).Unix())
	if err != nil {
		return fmt.Errorf("spending the form token: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("spending the form token: %w", err)
	}
	if affected == 0 {
		return ErrUsed
	}
	return nil
}

// PurgeTokens drops what can no longer be replayed. Without it the table is a
// row per submission for ever, which is a slow way to fill a disk.
func (g Guard) PurgeTokens(now time.Time) error {
	_, err := g.DB.Exec(`DELETE FROM form_token WHERE expires_at < ?`, now.Unix())
	return err
}

// --- the circuit breaker ----------------------------------------------------

// Accepting says whether a board is still taking submissions.
//
// This is the layer that protects attention rather than data. Everything else
// makes a flood expensive; this one puts a ceiling on how much of it can reach
// a person in an hour, and gives the operator something to point at instead of
// watching a queue fill.
func (g Guard) Accepting(repoID int64, now time.Time) (bool, error) {
	var recent int
	err := g.DB.QueryRow(`SELECT count(*) FROM item WHERE repo_id = ? AND created_at > ?`,
		repoID, now.Add(-time.Hour).Unix()).Scan(&recent)
	if err != nil {
		return false, fmt.Errorf("counting recent submissions: %w", err)
	}
	return recent < HourlyLimit, nil
}

func (g Guard) sign(payload string) []byte {
	mac := hmac.New(sha256.New, g.Key)
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:4])
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
