package fatalerror_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/ui/fatalerror"
)

// TestBodyWrapsAtSpacesAndKeepsAFlagWhole: the body names the flags that open
// a store. A wrap that also breaks after a hyphen put "--" at the end of one
// line and "store-name" at the start of the next.
func TestBodyWrapsAtSpacesAndKeepsAFlagWhole(t *testing.T) {
	t.Parallel()

	const body = "No task-manager store resolved for this directory.\n\n" +
		"Run 'taskmgr init' to create one, use --cwd to point at a directory that has one, " +
		"or use --store-name to open a central store by name."

	for width := 30; width <= 140; width++ {
		view := fatalerror.Render(fatalerror.State{Title: "title", Body: body, Width: width, Height: 24})

		lines := strings.Split(view, "\n")
		for _, line := range lines {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("width %d: a line is %d cells wide: %q", width, got, line)
			}
			if text := strings.TrimSpace(line); strings.HasSuffix(text, "-") {
				t.Errorf("width %d: a line ends inside a hyphenated word: %q", width, text)
			}
		}
		for _, whole := range []string{"--cwd", "--store-name", "task-manager"} {
			if !strings.Contains(view, whole) {
				t.Errorf("width %d: %q is cut in two:\n%s", width, whole, view)
			}
		}
	}

	// A word wider than the screen is the one thing still broken, so that no
	// line runs through the margin.
	long := strings.Repeat("x", 50)
	for _, line := range strings.Split(fatalerror.Render(fatalerror.State{Title: "title", Body: "see " + long, Width: 30, Height: 24}), "\n") {
		if got := lipgloss.Width(line); got > 30 {
			t.Fatalf("a line with a word wider than the screen is %d cells wide: %q", got, line)
		}
	}
}

func TestViewContainsExpectedContent(t *testing.T) {
	t.Parallel()

	view := fatalerror.Render(fatalerror.State{
		Title:  "task manager is not available",
		Body:   "The task-manager backend could not be initialized.",
		Width:  80,
		Height: 24,
	})

	checks := []string{"task manager is not available", "task-manager", "q"}
	for _, want := range checks {
		if !strings.Contains(view, want) {
			t.Errorf("expected %q in Render(80,24), output:\n%s", want, view)
		}
	}
}

func TestViewNoDatabaseShowsTailoredContent(t *testing.T) {
	t.Parallel()

	view := fatalerror.Render(fatalerror.State{
		Title:  "no task-manager store here",
		Body:   "No .tasks store was found in this directory.",
		Width:  80,
		Height: 24,
	})

	if !strings.Contains(view, "no task-manager store here") {
		t.Errorf("expected title in no-database view, got:\n%s", view)
	}
	if !strings.Contains(view, "No .tasks store") {
		t.Errorf("expected body in no-database view, got:\n%s", view)
	}
}

func TestViewZeroDimensionsDoesNotPanic(t *testing.T) {
	t.Parallel()

	view := fatalerror.Render(fatalerror.State{Title: "title", Body: "body"})

	if !strings.Contains(view, "title") {
		t.Errorf("expected title in Render(0,0), output:\n%s", view)
	}
}
