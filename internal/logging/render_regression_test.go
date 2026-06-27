package logging

// render_regression_test.go — regression guard for the stderr-suppression fix
// (task-manager-ui-o7tk, Bug B root cause).
//
// Root cause: when the app started, slog warn-level writes were mirrored to
// stderr, which corrupted the alt-screen TTY because Bubble Tea does not expect
// interleaved stderr writes during an active alt-screen session.
//
// Fix: Manager.SetStderrSuppressed(true) is called immediately before
// tea.Program.Run() and is reverted after Run returns.  The stderrHandler
// checks this flag in Enabled and Handle so that no stderr writes occur while
// the TUI is active.
//
// If the suppression mechanism is ever removed (e.g. SetStderrSuppressed
// becomes a no-op, or the flag check is deleted from stderrHandler.Enabled),
// TestStderrSuppressionBlocksWarnDuringActiveSession will fail with:
//
//	"regression: warn reached stderr while suppressed — alt-screen corruption
//	would occur"

import (
	"bytes"
	"strings"
	"testing"
)

// TestStderrSuppressionBlocksWarnDuringActiveSession is the regression guard
// for the o7tk Bug B fix.  It verifies that Manager.SetStderrSuppressed(true)
// fully silences slog warn-level writes to stderr.
//
// Failure condition: if suppression is removed or bypassed, the stderr buffer
// will contain the warn message that was written while suppressed.
func TestStderrSuppressionBlocksWarnDuringActiveSession(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	var stderr bytes.Buffer

	m := New(Options{
		StateDir:     stateDir,
		Stderr:       &stderr,
		SessionID:    "regrtest-b",
		ProjectRoot:  "/tmp/test",
		BuildVersion: "dev",
	})
	t.Cleanup(func() { _ = m.Close() })

	// Confirm that without suppression a warn does reach stderr (baseline).
	m.Logger().Warn("baseline-warn")
	if !strings.Contains(stderr.String(), "baseline-warn") {
		t.Fatalf("baseline: expected warn to reach stderr before suppression, got %q", stderr.String())
	}

	// Activate suppression — simulates the window immediately before
	// tea.Program.Run() where the alt-screen session begins.
	m.SetStderrSuppressed(true)
	stderr.Reset()

	// This write must NOT reach stderr.
	m.Logger().Warn("active-session-warn")

	// Regression assertion: if this fires, the suppression fix has been removed.
	if strings.Contains(stderr.String(), "active-session-warn") {
		t.Fatalf("regression (o7tk Bug B): warn reached stderr while suppressed — "+
			"alt-screen TTY corruption would occur during an active Bubble Tea session; "+
			"got %q", stderr.String())
	}

	// After suppression is lifted (simulates post-Run cleanup), writes resume.
	m.SetStderrSuppressed(false)
	m.Logger().Warn("post-session-warn")
	if !strings.Contains(stderr.String(), "post-session-warn") {
		t.Fatalf("expected warn to resume after suppression lifted, got %q", stderr.String())
	}
}
