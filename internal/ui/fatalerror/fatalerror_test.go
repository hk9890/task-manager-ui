package fatalerror_test

import (
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/ui/fatalerror"
)

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
