package web

import (
	"log/slog"
	"net/http"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/guard"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/util"
)

// StoplistSetting holds the words this operator does not want, comma separated.
// Their board, their vocabulary: a word that means nothing here is somebody
// else's daily traffic.
const StoplistSetting = "stoplist"

func (s *Server) guard() guard.Guard {
	return guard.Guard{DB: s.db, Key: s.formKey}
}

// addressOf returns the address to attribute a request to, and the block it
// sits in. Both, because an address alone means nothing in IPv6 and a block
// alone punishes a whole office for one person.
func (s *Server) addressOf(r *http.Request) (string, string) {
	address := util.ClientIP(r, s.cfg.TrustProxy, s.cfg.TrustProxyHops)
	return address, util.Bucket(address)
}

func (s *Server) bucketOf(r *http.Request) string {
	_, block := s.addressOf(r)
	return block
}

func (s *Server) stoplist() []string {
	raw, err := s.settings().Get(StoplistSetting)
	if err != nil || raw == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

// refused counts a refusal, by layer.
//
// The counter is the only way to know whether any of this is working. A layer
// that never refuses anything is a layer to delete; one that refuses nearly
// everything is biting people who did nothing wrong, and without the count
// neither is visible until somebody complains — and the people a spam filter
// wrongly refuses are exactly the people who do not complain, they leave.
func (s *Server) refused(repoID int64, layer string) {
	if layer == "" {
		layer = "unknown"
	}
	if _, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, '1')
		ON CONFLICT (key) DO UPDATE SET value = CAST(CAST(value AS INTEGER) + 1 AS TEXT)`,
		refusalKey(repoID, layer)); err != nil {
		slog.Error("counting a refusal", "error", err, "layer", layer)
	}
}

func refusalKey(repoID int64, layer string) string {
	return "refused." + itoa(repoID) + "." + layer
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
