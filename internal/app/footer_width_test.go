package app

// The footer legend's fit rule, asserted at the widths that decide it. The
// app-level goldens render at 80, 120 and 180 only, so the widths where a hint
// is dropped were never exercised.
//
// Direct assertions rather than goldens: the property is which hints survive,
// and more snapshots would churn on every unrelated header or footer edit.

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// footerSeparator stands between two hints of the legend, and footerCut ends
// a legend that dropped hints.
const (
	footerSeparator = " • "
	footerCut       = " …"
)

// TestFooterLegendDropsTrailingHintsRatherThanOverflow pins the legend's fit
// rule for every mode that has its own footer: a legend that does not fit loses
// hints from its end, whole, marks the drop, and never reaches past the
// terminal.
//
// One column too many is a footer the terminal wraps onto a second line, which
// the workspace height did not leave room for.
func TestFooterLegendDropsTrailingHintsRatherThanOverflow(t *testing.T) {
	t.Parallel()

	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}

	for _, active := range []mode.ID{mode.Board, mode.Docs, mode.Detail} {
		hints := footerHints(active, keys)
		full := styles.KeyLegend(hints, 0)
		fullWidth := lipgloss.Width(full)
		if got := strings.Count(full, footerSeparator); got != len(hints)-1 {
			t.Fatalf("%s: the unbounded legend joins %d hints with %d separators: %q", active, len(hints), got, full)
		}

		// One column either side of the full legend decides whether its last
		// hint is drawn. The widths are derived from the rendered legend, not
		// from the hints, so the test measures what the operator sees.
		if got := styles.KeyLegend(hints, fullWidth); got != full {
			t.Errorf("%s at width %d, exactly the legend's: got %q, want all of %q", active, fullWidth, got, full)
		}
		oneShort := styles.KeyLegend(hints, fullWidth-1)
		if got := strings.Count(oneShort, footerSeparator); got != len(hints)-2 {
			t.Errorf("%s at width %d, one short: %d separators, want the last hint and only it dropped: %q",
				active, fullWidth-1, got, oneShort)
		}

		previous := 0
		for width := 1; width <= fullWidth+5; width++ {
			legend := styles.KeyLegend(hints, width)
			if strings.Contains(legend, "\n") {
				t.Fatalf("%s at width %d: the legend takes more than one line: %q", active, width, legend)
			}
			cells := lipgloss.Width(legend)
			if cells > width {
				t.Fatalf("%s at width %d: the legend is %d cells wide: %q", active, width, cells, legend)
			}
			// Dropped from the end, whole: what is left is the start of the
			// full legend, and it ends on a hint or on the mark of the drop,
			// not on a separator. Only the first hint is cut, where not even
			// it fits.
			kept := strings.TrimSuffix(legend, footerCut)
			if first, _, _ := strings.Cut(full, footerSeparator); width < lipgloss.Width(first) {
				if !strings.HasSuffix(legend, "…") {
					t.Fatalf("%s at width %d: the first hint is not cut to fit: %q", active, width, legend)
				}
			} else if !strings.HasPrefix(full, kept) {
				t.Fatalf("%s at width %d: %q is not the start of %q", active, width, legend, full)
			} else if kept == legend && kept != full && lipgloss.Width(kept)+lipgloss.Width(footerCut) <= width {
				t.Fatalf("%s at width %d: the legend dropped hints and has room for the mark, but draws none: %q", active, width, legend)
			}
			if strings.HasSuffix(kept, strings.TrimRight(footerSeparator, " ")) {
				t.Fatalf("%s at width %d: the legend ends on a separator: %q", active, width, legend)
			}
			if cells < previous {
				t.Fatalf("%s: the legend shrank from %d to %d cells as the terminal grew to %d", active, previous, cells, width)
			}
			previous = cells
		}
		if previous != fullWidth {
			t.Errorf("%s: the widest legend drawn is %d cells, want the full %d", active, previous, fullWidth)
		}
	}
}

// TestFooterIsOneLineNoWiderThanTheTerminal renders the footer the shell
// draws, on every surface with a legend of its own, across the widths a
// terminal is resized through.
func TestFooterIsOneLineNoWiderThanTheTerminal(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready", "task", 1)
	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}
	m := mustNewModel(t, services)

	for _, active := range []mode.ID{mode.Board, mode.Docs, mode.Detail} {
		m.active = active
		for _, missing := range []bool{false, true} {
			m.projectRootMissing = missing
			for width := 1; width <= 200; width++ {
				m.width = width
				footer := m.renderFooter()
				if got := lipgloss.Height(footer); got != 1 {
					t.Fatalf("%s at width %d: the footer is %d lines: %q", active, width, got, footer)
				}
				if got := lipgloss.Width(footer); got > width {
					t.Fatalf("%s at width %d: the footer is %d cells wide: %q", active, width, got, footer)
				}
			}
		}
	}
}

// TestFooterKeepsTheLaunchersOffNoticeAtANarrowWidth pins where the notice
// stands: first, so the hints behind it are what a narrow terminal drops. At
// the end of the legend it was the first thing to go, on the one surface where
// the launch keys are pressed.
func TestFooterKeepsTheLaunchersOffNoticeAtANarrowWidth(t *testing.T) {
	t.Parallel()

	const notice = "launchers off: project path missing"

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready", "task", 1)
	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}
	m := mustNewModel(t, services)
	m.active = mode.Detail
	m.projectRootMissing = true

	m.width = 200
	wide := m.renderFooter()
	if !strings.HasPrefix(wide, " "+notice+footerSeparator) {
		t.Fatalf("the notice does not lead the legend: %q", wide)
	}
	hints := footerHints(mode.Detail, m.keys)
	if got := strings.Count(wide, footerSeparator); got != len(hints) {
		t.Fatalf("at width 200 the legend holds %d hints behind the notice, want all %d: %q", got, len(hints), wide)
	}

	// Room for the notice and one hint behind it, and no more.
	m.width = lipgloss.Width(notice) + 16
	narrow := m.renderFooter()
	if !strings.HasPrefix(narrow, " "+notice) {
		t.Fatalf("at width %d the notice is gone: %q", m.width, narrow)
	}
	if strings.Count(narrow, footerSeparator) >= strings.Count(wide, footerSeparator) {
		t.Fatalf("at width %d no hint was dropped: %q", m.width, narrow)
	}
	if !strings.HasPrefix(wide, strings.TrimSuffix(narrow, footerCut)) {
		t.Fatalf("at width %d the legend %q is not the start of %q", m.width, narrow, wide)
	}

	// The legend starts one cell in, so the notice needs that cell too.
	m.width = lipgloss.Width(notice) + 1
	if got := m.renderFooter(); got != " "+notice {
		t.Fatalf("at width %d, exactly the notice's: got %q, want %q", m.width, got, " "+notice)
	}

	// The notice belongs to Detail: no other surface launches anything.
	m.active = mode.Board
	m.width = 200
	if got := m.renderFooter(); strings.Contains(got, notice) {
		t.Fatalf("the board's legend carries the notice: %q", got)
	}
}
