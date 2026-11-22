package web

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/settings"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/notify"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/updater"
	"git.codigosh.com/CodigoSH/LastHelp/internal/version"
)

// Telling the administrators that a newer version is out.
//
// The server never looked for one unless somebody opened the panel, which is how
// a security fix sits for weeks beside an administrator who only ever reads the
// queue. This asks the forge on its own, and says so through the channel the
// administrator already set for the server: a push to their browsers, or whatever
// they chose. It is off until somebody turns it on, and each version is said once.

var (
	// noticeEvery is how often the forge is asked while somebody listens. An hour,
	// not a day: a notice that arrives a day after the release is poor for the one
	// kind of release that matters.
	noticeEvery = time.Hour
	// noticeFirstCheck is the wait after starting, so the server is up before it
	// goes looking.
	noticeFirstCheck = 30 * time.Second
)

// updateNoticeOn says whether an administrator asked to be told.
func (s *Server) updateNoticeOn() bool {
	value, err := s.settings().Get(settings.UpdateNotice)
	return err == nil && value == "on"
}

// betaChannel is the channel this installation follows: the one chosen, or the
// one its own version came from.
func (s *Server) betaChannel() bool {
	channel, err := s.settings().Get(settings.UpdateChannel)
	if err != nil {
		return false
	}
	return followsBeta(channel, version.Current)
}

// WatchUpdates asks for a newer version now and then, for as long as somebody
// listens, until the context ends. Run beside Work.
func (s *Server) WatchUpdates(ctx context.Context) {
	s.watching.Store(true)
	defer s.watching.Store(false)

	timer := time.NewTimer(noticeFirstCheck)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		s.notifyUpdate(ctx)
		timer.Reset(noticeEvery)
	}
}

// notifyUpdateSoon is for the moment somebody turns the notice on: without it a
// version that is already out would wait for the next hour. Only when the watcher
// is running, so a test that saves the setting does not reach for the forge.
func (s *Server) notifyUpdateSoon() {
	if !s.watching.Load() {
		return
	}
	go s.notifyUpdate(context.Background())
}

// notifyUpdate tells the administrators about the newest version if they asked
// to be told, there is a channel to tell them through, the version is newer than
// the one running and they have not been told of it.
//
// The version is marked as told only once the channel took it. A channel that was
// down, or a push with nobody subscribed, leaves it to be tried again, and with
// nobody listening the forge is not even asked.
func (s *Server) notifyUpdate(ctx context.Context) {
	// One at a time: the hourly check and the one when somebody turns it on can
	// meet, and would say the same thing twice.
	s.noticeMu.Lock()
	defer s.noticeMu.Unlock()

	if s.cfg.Offline || !s.updateNoticeOn() {
		return
	}
	channel := s.serverChannel()
	if !channel.Active() {
		return
	}
	// A build that cannot verify an update still has an update to be told about in a
	// container, where the answer is to pull an image; anywhere else it could not install it.
	if len(version.ReleaseKeys) == 0 && !inContainer() {
		return
	}

	checking, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	release, err := s.updater().Latest(checking, s.betaChannel())
	if err != nil {
		if !errors.Is(err, updater.ErrNoRelease) {
			slog.Warn("checking for a newer version to tell the administrators about", "error", err)
		}
		return
	}
	if !s.updater().Available(release) {
		return
	}
	if told, err := s.settings().Get(settings.UpdateNotified); err == nil && told == release.Tag {
		return
	}

	link := ""
	if base := strings.TrimSuffix(s.cfg.PublicURL, "/"); base != "" {
		link = base + "/panel/about"
	}
	msg := notify.Message{
		Title:   s.say("en", "notify.updateTitle", map[string]string{"version": release.Tag}),
		Body:    s.say("en", "notify.updateBody", map[string]string{"current": version.Current}),
		URL:     link,
		Kind:    "update",
		Admins:  true,
		Urgency: "low",
		Version: release.Tag,
		Current: version.Current,
	}
	sending, stop := context.WithTimeout(ctx, 20*time.Second)
	defer stop()
	if err := s.notifier().Send(sending, channel, msg); err != nil {
		slog.Warn("telling the administrators about a newer version", "kind", string(channel.Kind), "error", err)
		return
	}
	if err := s.settings().Set(settings.UpdateNotified, release.Tag); err != nil {
		slog.Error("remembering which version the administrators were told about", "error", err)
	}
}
