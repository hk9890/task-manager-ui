package toaster

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestOverlayNeverDrawsWiderThanTheTerminal pins the invariant the toast box has
// to hold: it is placed by overlay.Place, which splices the foreground line into
// the background unclipped, so a message wider than the terminal made the
// terminal soft-wrap the box — the right border went missing and the wrapped
// remainder pushed the rest of the frame down.
//
// Every style is checked because the severity glyphs differ in width.
func TestOverlayNeverDrawsWiderThanTheTerminal(t *testing.T) {
	t.Parallel()

	const width = 80
	background := strings.Repeat(strings.Repeat(".", width)+"\n", 12)

	messages := map[string]string{
		"repository error": `update issue failed: /home/operator/.taskmgr/stores/task-manager-ui/.tasks/config.yaml: the "hooks" key was withdrawn`,
		"editor failure":   "Failed to edit issue tm-1234: missing marker \"<!-- TASKMGRUI:FIELD:ASSIGNEE:BEGIN -->\"; your edits are kept at /tmp/taskmgr-ui-issue-tm-1234-2551491995.md",
		"wide runes":       strings.Repeat("課題", 90),
	}

	for _, style := range []Style{StyleSuccess, StyleError, StyleInfo, StyleWarn} {
		for name, message := range messages {
			m := New().Show(message, style)
			for _, line := range strings.Split(m.Overlay(background, width, 12), "\n") {
				if w := lipgloss.Width(line); w > width {
					t.Errorf("style %v, %s: rendered line is %d cells wide, want at most %d: %q",
						style, name, w, width, line)
				}
			}
		}
	}
}

// TestOverlayKeepsAShortMessageWhole guards the clip from over-reaching: a
// message that fits is not truncated and keeps its box.
func TestOverlayKeepsAShortMessageWhole(t *testing.T) {
	t.Parallel()

	const width = 80
	background := strings.Repeat(strings.Repeat(".", width)+"\n", 6)

	m := New().Show("Updated issue tm-7", StyleSuccess)
	got := m.Overlay(background, width, 6)

	if !strings.Contains(got, "Updated issue tm-7") {
		t.Errorf("short message was altered: %q", got)
	}
	if strings.Contains(got, "…") {
		t.Errorf("short message was truncated: %q", got)
	}
}
