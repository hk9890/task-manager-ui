package launcher

import (
	"os"
	"slices"
	"testing"
)

// TestNewDetachedCommandRequestsANewSession pins the detachment flag on the
// command Run starts. Detachment has no observable effect until a signal
// arrives, so this is the only assertion the fast gate can make about it; the
// signal itself is proven in the Tier-2 test.
//
// Without this, deleting the setsid call leaves both tiers green and the
// operator finds out when quitting taskmgr-ui also kills the editor it
// launched.
func TestNewDetachedCommandRequestsANewSession(t *testing.T) {
	t.Parallel()

	cmd := newDetachedCommand("true", nil, "", nil)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil: the launched process would stay in taskmgr-ui's process group")
	}
	if !cmd.SysProcAttr.Setsid {
		t.Error("Setsid is false: a signal to taskmgr-ui's process group would reach the launched tool")
	}
}

func TestNewDetachedCommandCarriesDirAndEnv(t *testing.T) {
	t.Parallel()

	t.Run("dir and env are applied", func(t *testing.T) {
		t.Parallel()

		cmd := newDetachedCommand("true", []string{"--flag"}, "/tmp", []string{"ISSUE_ID=tm-1"})

		if cmd.Dir != "/tmp" {
			t.Errorf("Dir = %q, want /tmp", cmd.Dir)
		}
		if !slices.Contains(cmd.Args, "--flag") {
			t.Errorf("Args = %v, want it to carry --flag", cmd.Args)
		}
		// Launcher env entries are added to the parent environment rather than
		// replacing it, so a launched tool keeps PATH, HOME and TERM.
		if !slices.Contains(cmd.Env, "ISSUE_ID=tm-1") {
			t.Errorf("Env does not carry the launcher entry: %v", cmd.Env)
		}
		if len(cmd.Env) <= 1 && len(os.Environ()) > 0 {
			t.Errorf("Env replaced the parent environment instead of extending it: %v", cmd.Env)
		}
	})

	t.Run("no env leaves the parent environment inherited", func(t *testing.T) {
		t.Parallel()

		cmd := newDetachedCommand("true", nil, "", nil)

		// A nil Env means exec inherits the parent's environment, which is the
		// documented behaviour when a launcher defines no env entries.
		if cmd.Env != nil {
			t.Errorf("Env = %v, want nil so the child inherits the parent environment", cmd.Env)
		}
	})
}
