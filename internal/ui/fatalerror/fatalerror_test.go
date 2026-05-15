package fatalerror_test

import (
	"strings"
	"testing"

	"github.com/hk9890/beads-workbench/internal/ui/fatalerror"
)

func TestViewContainsExpectedContent(t *testing.T) {
	t.Parallel()

	view := fatalerror.View("beads is not available", "The bd CLI tool was not found in your PATH.", 80, 24)

	checks := []string{"beads is not available", "bd", "q"}
	for _, want := range checks {
		if !strings.Contains(view, want) {
			t.Errorf("expected %q in View(80,24), output:\n%s", want, view)
		}
	}
}

func TestViewNoDatabaseShowsTailoredContent(t *testing.T) {
	t.Parallel()

	view := fatalerror.View("no beads project here", "No beads database was found in this directory.", 80, 24)

	if !strings.Contains(view, "no beads project here") {
		t.Errorf("expected title in no-database view, got:\n%s", view)
	}
	if !strings.Contains(view, "No beads database") {
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
