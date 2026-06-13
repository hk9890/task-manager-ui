package fatalerror_test

import (
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/ui/fatalerror"
)

func TestViewContainsExpectedContent(t *testing.T) {
	t.Parallel()

	view := fatalerror.View("task manager is not available", "The task-manager backend could not be initialized.", 80, 24)

	checks := []string{"task manager is not available", "task-manager", "q"}
	for _, want := range checks {
		if !strings.Contains(view, want) {
			t.Errorf("expected %q in View(80,24), output:\n%s", want, view)
		}
	}
}

func TestViewNoDatabaseShowsTailoredContent(t *testing.T) {
	t.Parallel()

	view := fatalerror.View("no task-manager store here", "No .tasks store was found in this directory.", 80, 24)

	if !strings.Contains(view, "no task-manager store here") {
		t.Errorf("expected title in no-database view, got:\n%s", view)
	}
	if !strings.Contains(view, "No .tasks store") {
		t.Errorf("expected body in no-database view, got:\n%s", view)
	}
}

func TestViewZeroDimensionsDoesNotPanic(t *testing.T) {
	t.Parallel()

	view := fatalerror.View("title", "body", 0, 0)

	if !strings.Contains(view, "title") {
		t.Errorf("expected title in View(0,0), output:\n%s", view)
	}
}
