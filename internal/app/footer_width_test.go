package app

// The footer's compact/verbose switch point, asserted at its own width. The
// app-level tests render at 80, 120 and 180 only, so 89 and 90 — the two
// widths the comparison actually distinguishes — were never exercised.
//
// Direct assertions rather than goldens: the property is which form is chosen,
// and two more snapshots would churn on every unrelated header or footer edit.

import (
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/mode"
)

// footerSeparator marks the verbose form: it spells each binding out and joins
// them with the field separator, where the compact form is bare key glyphs
// separated by spaces. Every mode follows that split, so this discriminates
// the two forms without hard-coding each mode's prose.
const footerSeparator = " · "

// TestFooterHelpTextSwitchesFormAtItsThreshold pins the breakpoint from both
// sides, for every mode that has its own footer. The widths are literal on
// purpose: reading them from the source constant would make the test move with
// it and assert nothing.
//
// One column either way decides whether the verbose footer is rendered into a
// terminal too narrow to hold it, which overflows the line.
func TestFooterHelpTextSwitchesFormAtItsThreshold(t *testing.T) {
	t.Parallel()

	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}

	for _, active := range []mode.ID{mode.Board, mode.Docs, mode.Search, mode.Detail} {
		compact := footerHelpText(active, 89, keys)
		if strings.Contains(compact, footerSeparator) {
			t.Errorf("%s at width 89 rendered the verbose footer: %q", active, compact)
		}

		verbose := footerHelpText(active, 90, keys)
		if !strings.Contains(verbose, footerSeparator) {
			t.Errorf("%s at width 90 rendered the compact footer: %q", active, verbose)
		}

		// The two forms must actually differ, or the assertions above would
		// pass on a footer that ignores width entirely.
		if compact == verbose {
			t.Errorf("%s renders an identical footer at 89 and 90: %q", active, compact)
		}
		if len(verbose) <= len(compact) {
			t.Errorf("%s: verbose footer (%d chars) is not longer than the compact one (%d)",
				active, len(verbose), len(compact))
		}
	}
}
