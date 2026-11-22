package web

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/config"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

// Two moderators marking two reports as spam in the same moment each read the
// list, each add their word, and the second write is the list without the first
// one's word: a word they chose to filter on is not on the list, and nothing
// says so.
func TestWordsAddedAtTheSameTimeAreAllKept(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	srv, err := New(config.Config{DBFile: dbPath, KeyFile: filepath.Join(filepath.Dir(dbPath), "test.key")}, db)
	if err != nil {
		t.Fatal(err)
	}

	const n = 20
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(word string) {
			defer wg.Done()
			<-start
			if failure := srv.addWord(word); failure != "" {
				t.Errorf("adding %s: %s", word, failure)
			}
		}(fmt.Sprintf("spamword%02d", i))
	}
	close(start)
	wg.Wait()

	if got := len(srv.currentWords()); got != n {
		t.Errorf("%d of %d words were kept: %v", got, n, srv.currentWords())
	}
}
