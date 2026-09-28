package web

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/settings"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/updater"
	"git.codigosh.com/CodigoSH/LastHelp/internal/version"
)

// ReleaseAPI and ReleaseDownloads are the one outbound address that is not the
// operator's choice, which is why SECURITY.md names it: the update path talks to
// our forge and to nothing else, with no intermediary.
const (
	ReleaseAPI       = "https://git.codigosh.com/api/v1/repos/CodigoSH/LastHelp"
	ReleaseDownloads = "https://git.codigosh.com/CodigoSH/LastHelp/releases/download"
)

func (s *Server) updater() updater.Updater {
	return updater.Updater{
		Out:          s.out,
		APIBase:      ReleaseAPI,
		DownloadBase: ReleaseDownloads,
		Keys:         version.ReleaseKeys,
		Current:      version.Current,
		SchemaVersion: func() (int, error) {
			return database.SchemaVersion(s.db)
		},
		Now: s.now,
	}
}

// inContainer says whether replacing the binary would achieve anything.
//
// Inside a container it would not: the image is what comes back on the next
// restart, so an in-place update lasts exactly until something restarts the
// container and then silently reverts. Telling somebody to pull a new image is
// the honest answer; letting them press a button that appears to work and
// quietly undoes itself is not.
func inContainer() bool { return inContainerUnder("/") }

// inContainerUnder is inContainer against a root that a test can build.
//
// Each engine leaves its own mark: Docker writes /.dockerenv and Podman
// /run/.containerenv. The cgroup of the first process used to name the engine
// too, and under cgroup v2 — the default on every current distribution — it
// reads "0::/" and names nothing, which is how an image run by Podman offered
// an update the next restart would have undone.
func inContainerUnder(root string) bool {
	for _, mark := range []string{".dockerenv", "run/.containerenv"} {
		if _, err := os.Stat(filepath.Join(root, mark)); err == nil {
			return true
		}
	}
	if cgroup, err := os.ReadFile(filepath.Join(root, "proc/1/cgroup")); err == nil {
		text := string(cgroup)
		return strings.Contains(text, "docker") || strings.Contains(text, "containerd") ||
			strings.Contains(text, "kubepods")
	}
	return false
}

// handleUpdateStatus says what is running and what could be.
func (s *Server) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}

	body := map[string]any{
		"current":    version.Current,
		"commit":     version.Commit,
		"container":  inContainer(),
		"can_verify": len(version.ReleaseKeys) > 0,
		"offline":    s.cfg.Offline,
	}

	// Which channel this installation follows, so the panel can show it rather
	// than keeping a copy of the answer and drifting from it.
	body["beta"] = s.wantsBeta(r)

	// Every reason it could not check, said plainly rather than as an empty
	// answer that looks like "you are up to date".
	switch {
	case s.cfg.Offline:
		body["reason"] = "error.update_offline"
	case len(version.ReleaseKeys) == 0:
		body["reason"] = "error.update_unverifiable"
	case inContainer():
		body["reason"] = "error.update_container"
	default:
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		release, err := s.updater().Latest(ctx, s.wantsBeta(r))
		switch {
		case errors.Is(err, updater.ErrNoRelease):
			// Reached, and there is nothing on this channel — which is what
			// every installation saw while only betas existed. Saying it could
			// not be reached sends somebody to check a firewall that is fine.
			body["reason"] = "error.update_none"
		case err != nil:
			slog.Warn("checking for updates", "error", err)
			body["reason"] = "error.update_unreachable"
		default:
			body["latest"] = release.Tag
			body["notes"] = release.Notes
			body["available"] = s.updater().Available(release)
		}
	}

	writeJSON(w, http.StatusOK, body)
}

// handleUpdatePerform installs a release and restarts.
//
// Server-level: replacing the binary is not something an organisation's owner
// does to the machine somebody else is also using.
func (s *Server) handleUpdatePerform(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}
	if inContainer() {
		writeError(w, http.StatusBadRequest, "error.update_container")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	release, err := s.updater().Latest(ctx, s.wantsBeta(r))
	if err != nil {
		writeError(w, http.StatusBadGateway, "error.update_unreachable")
		return
	}
	if !s.updater().Available(release) {
		writeError(w, http.StatusBadRequest, "error.update_none")
		return
	}

	steps := make([]string, 0, 8)
	err = s.updater().Apply(ctx, release, s.cfg.DBFile, func(step updater.Step, detail string) {
		slog.Info("update", "step", string(step), "detail", detail)
		steps = append(steps, string(step))
	})
	if err != nil {
		slog.Error("the update was refused", "error", err, "release", release.Tag)
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": keyForUpdateError(err),
			"steps": steps,
		})
		return
	}

	slog.Info("updated; restarting", "release", release.Tag)

	// Answered before restarting, or the person who pressed the button gets a
	// severed connection and no way to tell success from failure.
	writeJSON(w, http.StatusOK, map[string]any{"installed": release.Tag, "steps": steps})
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	go restart()
}

// restart replaces this process with the binary now on disk.
//
// Re-executing ourselves with our own path and our own arguments — nothing here
// comes from a request. If exec fails there is nothing sensible left to do but
// exit and let whatever supervises this start it again, which is what a systemd
// unit with Restart=on-failure is for.
//
// Which is why it exits NON-ZERO. This said os.Exit(0) while the comment above
// it said what it says now, and the two disagreed in the worst possible place:
// `Restart=on-failure` does not restart a unit that exited successfully. The
// operator pressed update, was told it worked, and the service stopped and
// stayed stopped — on the one path that runs when an update has already gone
// wrong.
func restart() {
	time.Sleep(500 * time.Millisecond)

	path, err := executablePath()
	if err != nil {
		slog.Error("could not find our own binary to restart", "error", err)
		exitProcess(1)
		return
	}
	if err := execSelf(path, os.Args, os.Environ()); err != nil {
		slog.Error("could not restart; exiting for the supervisor", "error", err)
		exitProcess(1)
		return
	}
}

// Variables so a test can watch what this decides. Calling the real ones in a
// test either replaces the test binary or ends the run, and the thing worth
// asserting is the exit code: it is what decides whether the service comes
// back.
var (
	executablePath = os.Executable
	execSelf       = syscall.Exec
	exitProcess    = os.Exit
)

func keyForUpdateError(err error) string {
	switch {
	case errors.Is(err, updater.ErrNoKeys):
		return "error.update_unverifiable"
	case errors.Is(err, updater.ErrUnsigned), errors.Is(err, updater.ErrBadSignature):
		return "error.update_bad_signature"
	case errors.Is(err, updater.ErrBadChecksum):
		return "error.update_bad_checksum"
	case errors.Is(err, updater.ErrWontRun):
		return "error.update_wont_run"
	case errors.Is(err, updater.ErrOlderThanDatabase):
		return "error.update_too_old"
	default:
		return "error.update_failed"
	}
}

// wantsBeta answers which channel this installation follows.
//
// The query parameter still wins, so a check can be made against the other
// channel without changing what the server follows — which is what "show me
// what a beta would be" needs.
//
// With no channel chosen, an installation follows the one it came from: a
// pre-release follows the betas. It used to follow the stable channel, and
// while only betas are published that channel answers "nothing here" — so a
// beta installation was told it was up to date with the next beta out.
func (s *Server) wantsBeta(r *http.Request) bool {
	if value := r.URL.Query().Get("beta"); value != "" {
		return value == "true"
	}
	channel, err := s.settings().Get(settings.UpdateChannel)
	if err != nil {
		return false
	}
	return followsBeta(channel, version.Current)
}

// followsBeta is the channel an installation follows: the one chosen, or with
// none chosen, the one its own version came from.
func followsBeta(channel, current string) bool {
	if channel == "" {
		return updater.Prerelease(current)
	}
	return channel == "beta"
}

type channelChange struct {
	Beta bool `json:"beta"`
}

// handleSetChannel remembers which channel to follow.
func (s *Server) handleSetChannel(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in channelChange
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	channel := "stable"
	if in.Beta {
		channel = "beta"
	}
	if err := s.settings().Set(settings.UpdateChannel, channel); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"beta": in.Beta})
}
