package web

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/updater"
)

// events reads a server-sent event stream back as "step:status" lines.
func events(t *testing.T, body string) []string {
	t.Helper()
	var out []string
	for _, frame := range strings.Split(strings.TrimSpace(body), "\n\n") {
		payload := strings.TrimPrefix(frame, "data: ")
		var e map[string]string
		if err := json.Unmarshal([]byte(payload), &e); err != nil {
			t.Fatalf("a frame that is not an event: %q", frame)
		}
		line := e["step"] + ":" + e["status"]
		if e["error"] != "" {
			line += ":" + e["error"]
		}
		out = append(out, line)
	}
	return out
}

// TestAnUpdateIsReportedStepByStep.
//
// The dialog's timeline is Lastboard's: every step goes running and then ok,
// in order, and the last event of a good update announces the restart. Each
// step of this updater that the dialog shows as one — the checksum list and
// its signature — is reported once.
func TestAnUpdateIsReportedStepByStep(t *testing.T) {
	rec := httptest.NewRecorder()
	installed := streamUpdate(rec, func(progress func(updater.Step, string)) error {
		for _, step := range []updater.Step{
			updater.StepChecksums, updater.StepSignature, updater.StepDownload, updater.StepVerify,
			updater.StepProve, updater.StepBackup, updater.StepInstall,
		} {
			progress(step, "")
		}
		return nil
	})
	if !installed {
		t.Fatal("a good update was reported as not installed")
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("answered as %q", got)
	}
	var want []string
	for _, step := range UpdateSteps[:len(UpdateSteps)-1] {
		want = append(want, step+":running", step+":ok")
	}
	want = append(want, "restart:running")
	if got := events(t, rec.Body.String()); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("the stream said\n  %v\nwant\n  %v", got, want)
	}
}

// TestAFailedStepSaysWhichAndWhy.
//
// A refusal stops the stream on the step that failed, with the reason the panel
// translates, and never announces a restart.
func TestAFailedStepSaysWhichAndWhy(t *testing.T) {
	rec := httptest.NewRecorder()
	installed := streamUpdate(rec, func(progress func(updater.Step, string)) error {
		progress(updater.StepChecksums, "")
		progress(updater.StepSignature, "")
		return updater.ErrBadSignature
	})
	if installed {
		t.Fatal("a refused update was reported as installed")
	}
	got := strings.Join(events(t, rec.Body.String()), " ")
	if want := "signature:running signature:error:error.update_bad_signature"; got != want {
		t.Errorf("the stream said %q, want %q", got, want)
	}
}
