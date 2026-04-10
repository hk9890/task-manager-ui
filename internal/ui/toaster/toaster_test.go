package toaster

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestShowHide(t *testing.T) {
	m := New()
	if m.Visible() {
		t.Fatalf("new toaster should not be visible")
	}

	m = m.Show("done", StyleSuccess)
	if !m.Visible() {
		t.Fatalf("toaster should be visible after Show")
	}
	if !strings.Contains(m.View(), "done") {
		t.Fatalf("toast view should contain message")
	}

	m = m.Hide()
	if m.Visible() {
		t.Fatalf("toaster should not be visible after Hide")
	}
}

func TestOverlayReturnsBackgroundWhenHidden(t *testing.T) {
	bg := "background"
	m := New()
	if got := m.Overlay(bg, 20, 5); got != bg {
		t.Fatalf("expected background unchanged when hidden")
	}
}

func TestViewStylesAreDistinctAndContainContent(t *testing.T) {
	t.Parallel()

	message := "gateway timeout"
	model := New()

	views := map[Style]string{
		StyleError:   model.Show(message, StyleError).View(),
		StyleWarn:    model.Show(message, StyleWarn).View(),
		StyleInfo:    model.Show(message, StyleInfo).View(),
		StyleSuccess: model.Show(message, StyleSuccess).View(),
	}

	for style, view := range views {
		if !strings.Contains(view, message) {
			t.Fatalf("style %v view should contain message %q, got %q", style, message, view)
		}
	}

	if views[StyleError] == views[StyleWarn] ||
		views[StyleError] == views[StyleInfo] ||
		views[StyleError] == views[StyleSuccess] ||
		views[StyleWarn] == views[StyleInfo] ||
		views[StyleWarn] == views[StyleSuccess] ||
		views[StyleInfo] == views[StyleSuccess] {
		t.Fatalf("expected all toast style views to be visually distinct")
	}
}

func TestOverlayUsesProvidedTerminalWidth(t *testing.T) {
	t.Parallel()

	const width = 50
	const height = 5

	bgLine := strings.Repeat(".", width)
	bg := strings.Join([]string{bgLine, bgLine, bgLine, bgLine, bgLine}, "\n")

	m := New().Show("Saved", StyleSuccess)
	overlaid := m.Overlay(bg, width, height)

	if !strings.Contains(overlaid, "Saved") {
		t.Fatalf("expected overlaid toast to contain message, got %q", overlaid)
	}

	for i, line := range strings.Split(overlaid, "\n") {
		if lipgloss.Width(line) != width {
			t.Fatalf("line %d width mismatch: want %d, got %d", i, width, lipgloss.Width(line))
		}
	}
}
