package fatalerror_test

import (
	"strings"
	"testing"

	"github.com/hk9890/beads-workbench/internal/ui/fatalerror"
)

func TestViewContainsExpectedContent(t *testing.T) {
	t.Parallel()

	view := fatalerror.View(80, 24)

	checks := []string{"beads is not available", "bd", "q"}
	for _, want := range checks {
		if !strings.Contains(view, want) {
			t.Errorf("expected %q in View(80,24), output:\n%s", want, view)
		}
	}
}

func TestViewZeroDimensionsDoesNotPanic(t *testing.T) {
	t.Parallel()

	view := fatalerror.View(0, 0)

	if !strings.Contains(view, "beads is not available") {
		t.Errorf("expected title in View(0,0), output:\n%s", view)
	}
}
