package item

import (
	"fmt"
	"sort"
)

// What a team looking after a queue needs to know that a count cannot say:
// how long the longest wait has been, whether the people who wrote were
// answered, and how quickly. All of it is read from what is already stored —
// nothing here is a second place the facts are kept.

// Facts is a filter's matches summed up: how many, since when the oldest has
// been there, and how much the most wanted of them is wanted.
type Facts struct {
	Count int
	// Oldest is when the oldest of them arrived, in seconds; zero when none did.
	Oldest    int64
	MostVoted int
}

// Facts reads the same conditions List does, from the same function, so what
// the overview says about a set and what the listing shows of it cannot
// disagree about what the set is.
func (s Store) Facts(f Filter) (Facts, error) {
	where, args := f.where()
	var out Facts
	err := s.DB.QueryRow(`SELECT count(*), COALESCE(MIN(created_at), 0), COALESCE(MAX(vote_count), 0)
		FROM item WHERE `+where, args...).Scan(&out.Count, &out.Oldest, &out.MostVoted)
	if err != nil {
		return Facts{}, fmt.Errorf("summing up items: %w", err)
	}
	return out, nil
}

// Replies is how the team answered what arrived since some moment.
type Replies struct {
	// Published is how many reports were published since then, and Answered how
	// many of them have a reply from the team.
	Published int
	Answered  int
	// Median is how long the first reply took, counted from when the report
	// became visible: moderation is a wait of its own and the reporter does not
	// feel it as the team being slow to answer. Zero when nothing was answered.
	Median int64
}

// FirstReplies measures the answers to what was published since `since`, on
// the given repositories. With none, nothing matches: the direction a mistake
// in who may see what should fail in.
func (s Store) FirstReplies(repoIDs []int64, since int64) (Replies, error) {
	var out Replies
	if len(repoIDs) == 0 {
		return out, nil
	}
	args := []any{string(Published), since}
	for _, id := range repoIDs {
		args = append(args, id)
	}
	rows, err := s.DB.Query(`SELECT
			(SELECT MIN(c.created_at) FROM comment c WHERE c.item_id = i.id AND c.author_kind = '`+string(AuthorTeam)+`'),
			COALESCE(i.published_at, i.created_at)
		FROM item i
		WHERE i.merged_into IS NULL AND i.moderation_state = ? AND i.created_at >= ?
		  AND i.repo_id IN (`+placeholders(len(repoIDs))+`)
		LIMIT 5000`, args...)
	if err != nil {
		return out, fmt.Errorf("measuring the replies: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var waits []int64
	for rows.Next() {
		var first *int64
		var visible int64
		if err := rows.Scan(&first, &visible); err != nil {
			return out, fmt.Errorf("measuring the replies: %w", err)
		}
		out.Published++
		if first == nil {
			continue
		}
		out.Answered++
		// A reply dated before the report was visible — a team member writing
		// on a report they were then asked to publish — took no time at all.
		waits = append(waits, max(*first-visible, 0))
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("measuring the replies: %w", err)
	}
	if len(waits) > 0 {
		sort.Slice(waits, func(a, b int) bool { return waits[a] < waits[b] })
		out.Median = waits[len(waits)/2]
	}
	return out, nil
}

// OldestWaiting is when the oldest thing waiting for a person arrived — a
// report or a comment — on the given repositories, or zero when nothing waits.
func (s Store) OldestWaiting(repoIDs []int64) (int64, error) {
	if len(repoIDs) == 0 {
		return 0, nil
	}
	in := placeholders(len(repoIDs))
	args := []any{string(Pending)}
	for _, id := range repoIDs {
		args = append(args, id)
	}
	args = append(args, string(Pending))
	for _, id := range repoIDs {
		args = append(args, id)
	}
	var oldest int64
	err := s.DB.QueryRow(`SELECT COALESCE(MIN(at), 0) FROM (
			SELECT created_at AS at FROM item
			 WHERE merged_into IS NULL AND moderation_state = ? AND repo_id IN (`+in+`)
			UNION ALL
			SELECT c.created_at FROM comment c JOIN item i ON i.id = c.item_id
			 WHERE c.moderation_state = ? AND i.repo_id IN (`+in+`))`, args...).Scan(&oldest)
	if err != nil {
		return 0, fmt.Errorf("finding the oldest wait: %w", err)
	}
	return oldest, nil
}

// NotOnForge is how many confirmed bugs have no issue behind them on a
// repository whose organisation has a forge: work the team agreed is real and
// that nobody has been given.
func (s Store) NotOnForge(repoIDs []int64, noForge string) (int, error) {
	if len(repoIDs) == 0 {
		return 0, nil
	}
	args := []any{string(Bug), string(StatusConfirmed), string(Published), noForge}
	for _, id := range repoIDs {
		args = append(args, id)
	}
	var n int
	err := s.DB.QueryRow(`SELECT count(*) FROM item i
		JOIN repository r ON r.id = i.repo_id
		JOIN organization o ON o.id = r.org_id
		WHERE i.merged_into IS NULL AND i.kind = ? AND i.status = ? AND i.moderation_state = ?
		  AND o.forge_kind <> ?
		  AND NOT EXISTS (SELECT 1 FROM issue_link l WHERE l.item_id = i.id)
		  AND i.repo_id IN (`+placeholders(len(repoIDs))+`)`, args...).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting bugs with no issue: %w", err)
	}
	return n, nil
}
