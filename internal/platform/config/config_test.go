package config

import (
	"os"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/handoff"
)

// The name of the variable a replaced process finds its socket under is written
// twice: where the new process reads it (here) and where the old one writes it
// (handoff). A name that drifts in one of them is a restart that closes the port
// again with nothing failing, so the two are held to each other.
func TestTheSocketHandedOverIsReadUnderTheNameItIsWrittenWith(t *testing.T) {
	t.Setenv(handoff.Env, "7")

	cfg := Load()

	if cfg.ListenFD != "7" {
		t.Errorf("ListenFD = %q: the variable handoff writes (%s) is not the one config reads", cfg.ListenFD, handoff.Env)
	}
	if _, left := os.LookupEnv(handoff.Env); left {
		t.Error("the variable was left for any process this one starts")
	}
}

func TestWithNoSocketHandedOverThereIsNone(t *testing.T) {
	t.Setenv(handoff.Env, "")
	if got := Load().ListenFD; got != "" {
		t.Errorf("ListenFD = %q, want none", got)
	}
}
