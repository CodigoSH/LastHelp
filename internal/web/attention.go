package web

import (
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
)

// attentionItem is one thing about this server its administrator should know
// and can fix. The page words it from the key, in the reader's language; the
// params are the facts the sentence needs.
type attentionItem struct {
	Key    string         `json:"key"`
	Tone   string         `json:"tone"` // "bad", "warn" or "info"
	Params map[string]any `json:"params,omitempty"`
}

// diskWarn and diskBad are how full the disk under the database may be before
// About says so: the same lines the disk's bar turns amber and red at.
const (
	diskWarn = 0.75
	diskBad  = 0.90
)

// handleAttention lists what needs the administrator, worst first. Only what
// this server can see for itself and somebody can act on: a problem, never a
// suggestion.
func (s *Server) handleAttention(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.attention()})
}

func (s *Server) attention() []attentionItem {
	out := []attentionItem{}

	if free, total, ok := diskFree(s.dataDir()); ok {
		if item, full := diskAttention(free, total); full {
			out = append(out, item)
		}
	}

	if failed, err := s.outbox().Abandoned(); err != nil {
		slog.Error("reading abandoned jobs", "error", err)
	} else if len(failed) > 0 {
		out = append(out, attentionItem{Key: "jobs", Tone: "warn", Params: map[string]any{
			"count": len(failed), "last_error": failed[len(failed)-1].LastErr,
		}})
	}

	if s.forwardedUnread.Load() {
		out = append(out, attentionItem{Key: "proxy", Tone: "warn", Params: map[string]any{"install": installKind()}})
	}

	// Without a public address the issues opened on a forge carry no link back
	// to the report, and only an organisation with a forge is affected.
	if s.cfg.PublicURL == "" {
		var forged int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM organization WHERE forge_kind <> ?`, string(org.ForgeNone)).Scan(&forged); err != nil {
			slog.Error("counting organisations with a forge", "error", err)
		} else if forged > 0 {
			out = append(out, attentionItem{Key: "public_url", Tone: "warn", Params: map[string]any{"install": installKind()}})
		}
	}

	// Information rather than a fault: a server on a home network answered over
	// plain HTTP is a choice, but it is one that leaves sessions on cookies that
	// are not marked secure, and some things a browser only does over HTTPS.
	//
	// Not while a proxy has announced itself and nobody declared it: behind one
	// the connection to this server is plain whatever the visitor used, and what
	// the visitor used is in a header this server is not yet told to believe. The
	// notice above says what to do, and this one is judged again once it is done.
	if s.insecureSeen.Load() && (s.cfg.TrustProxy || !s.forwardedUnread.Load()) {
		out = append(out, attentionItem{Key: "https", Tone: "info"})
	}
	return out
}

// diskAttention says whether a disk this full needs the administrator, and how
// badly.
func diskAttention(free, total uint64) (attentionItem, bool) {
	if total == 0 || free > total {
		return attentionItem{}, false
	}
	used := float64(total-free) / float64(total)
	if used < diskWarn {
		return attentionItem{}, false
	}
	tone := "warn"
	if used >= diskBad {
		tone = "bad"
	}
	return attentionItem{Key: "disk", Tone: tone, Params: map[string]any{"free_bytes": free, "total_bytes": total}}, true
}

// dataDir is the folder the database lives in, which is the disk that matters.
func (s *Server) dataDir() string {
	dir := filepath.Dir(s.cfg.DBFile)
	if dir == "" {
		return "."
	}
	return dir
}

// noticeInsecure remembers that somebody signed in used this server over plain
// HTTP from another machine. This machine itself does not count: that is
// somebody trying it out, or a proxy on the same host.
//
// A person, not a request: a proxy's health probe, a check of this very server
// by its own address or the restart after an update all arrive without HTTPS and
// are nobody using it. It sits inside the authenticator, which is what says
// whether somebody is signed in.
func (s *Server) noticeInsecure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.insecureSeen.Load() && !s.isSecure(r) && !fromLoopback(r) {
			if _, signedIn := middleware.ActorFrom(r); signedIn && s.insecureSeen.CompareAndSwap(false, true) {
				slog.Info("somebody signed in is using LastHelp over plain HTTP", "from", r.RemoteAddr, "path", r.URL.Path)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// installKind says how this server is installed, because what to do about a
// setting depends on it: a container's environment is changed where the
// container is defined, a native installation's in its configuration file.
func installKind() string {
	if inContainer() {
		return "container"
	}
	return "native"
}

func fromLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// hostStats is the machine this runs on, as Linux reports it: the processor's
// use over a short sample and the memory in use. Inside a container these are
// what the container is given, which is what the operator can change. On any
// other system there is nothing to read, and the caller leaves them out.
type hostStats struct {
	CPUPercent float64 `json:"cpu_percent"`
	Cores      int     `json:"cores"`
	MemTotal   uint64  `json:"mem_total_bytes"`
	MemUsed    uint64  `json:"mem_used_bytes"`
}

func readHostStats(sample time.Duration) (hostStats, bool) {
	if runtime.GOOS != "linux" {
		return hostStats{}, false
	}
	total, used, err := readMemInfo()
	if err != nil {
		return hostStats{}, false
	}
	cpu, err := cpuUsage(sample)
	if err != nil {
		return hostStats{}, false
	}
	return hostStats{CPUPercent: cpu, Cores: runtime.NumCPU(), MemTotal: total, MemUsed: used}, true
}

// readMemInfo is the memory in use: total less what is available, which counts
// the page cache as free, as the kernel itself does.
func readMemInfo() (total, used uint64, err error) {
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	var free, avail uint64
	for _, line := range strings.Split(string(raw), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		kb, _ := strconv.ParseUint(f[1], 10, 64)
		switch f[0] {
		case "MemTotal:":
			total = kb * 1024
		case "MemFree:":
			free = kb * 1024
		case "MemAvailable:":
			avail = kb * 1024
		}
	}
	if total == 0 {
		return 0, 0, os.ErrNotExist
	}
	if avail == 0 {
		avail = free
	}
	return total, total - avail, nil
}

// cpuUsage samples /proc/stat twice and says how much of the time between was
// not idle.
func cpuUsage(sample time.Duration) (float64, error) {
	read := func() (idle, total uint64, err error) {
		raw, err := os.ReadFile("/proc/stat")
		if err != nil {
			return 0, 0, err
		}
		line, _, _ := strings.Cut(string(raw), "\n")
		f := strings.Fields(line)
		if len(f) < 5 || f[0] != "cpu" {
			return 0, 0, os.ErrInvalid
		}
		for i, v := range f[1:] {
			n, _ := strconv.ParseUint(v, 10, 64)
			total += n
			if i == 3 || i == 4 { // idle and iowait
				idle += n
			}
		}
		return idle, total, nil
	}
	idle1, total1, err := read()
	if err != nil {
		return 0, err
	}
	time.Sleep(sample)
	idle2, total2, err := read()
	if err != nil {
		return 0, err
	}
	if total2 <= total1 {
		return 0, nil
	}
	busy := 1 - float64(idle2-idle1)/float64(total2-total1)
	return busy * 100, nil
}
