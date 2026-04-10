package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/beads-workbench/internal/mode"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
)

var startupBoardRequiredSnippets = []string{"Default", "Not Ready", "Ready", "In Progress"}

var obviousRuntimeErrorSnippets = []string{
	"Error: blocked issues:",
	"Error: ready issues:",
	"Error: list issues:",
	"Search failed",
	"exclusive lock",
	"panic:",
}

// AssertContainsAll verifies that every snippet exists in the rendered output.
func AssertContainsAll(tb testing.TB, output string, snippets ...string) {
	tb.Helper()

	for _, snippet := range snippets {
		if !strings.Contains(output, snippet) {
			tb.Fatalf("expected output to contain %q, got:\n%s", snippet, output)
		}
	}
}

// AssertNotContainsAny verifies that none of the snippets exist in output.
func AssertNotContainsAny(tb testing.TB, output string, snippets ...string) {
	tb.Helper()

	for _, snippet := range snippets {
		if strings.Contains(output, snippet) {
			tb.Fatalf("expected output to not contain %q, got:\n%s", snippet, output)
		}
	}
}

// AssertStartupBoardLayoutSanity checks visible startup lane/layout markers.
func AssertStartupBoardLayoutSanity(tb testing.TB, output string) {
	tb.Helper()

	AssertContainsAll(tb, output, startupBoardRequiredSnippets...)
	if strings.Count(output, "│") < 5 {
		tb.Fatalf("expected board layout separators in startup output, got:\n%s", output)
	}
}

// AssertNoObviousRuntimeErrorPanels checks for common runtime error panel text.
func AssertNoObviousRuntimeErrorPanels(tb testing.TB, output string) {
	tb.Helper()

	AssertNotContainsAny(tb, output, obviousRuntimeErrorSnippets...)
}

// AssertLatestSearchQueryText verifies the latest search query text sent to gateway.
func AssertLatestSearchQueryText(tb testing.TB, calls []fakes.GatewayCall, want string) {
	tb.Helper()

	for i := len(calls) - 1; i >= 0; i-- {
		call := calls[i]
		if call.Method != fakes.MethodSearchIssues {
			continue
		}

		input, ok := call.Input.(fakes.SearchIssuesCall)
		if !ok {
			tb.Fatalf("expected SearchIssuesCall payload, got %T", call.Input)
		}
		if input.Query.Text != want {
			tb.Fatalf("expected latest search query %q, got %q", want, input.Query.Text)
		}
		return
	}

	tb.Fatalf("expected at least one search gateway call, got %#v", calls)
}

// AssertActionRequest checks action request messages for scenario navigation tests.
func AssertActionRequest(tb testing.TB, msg tea.Msg, wantMode mode.ID, wantAction mode.Action) {
	tb.Helper()

	action, ok := msg.(mode.ActionRequestMsg)
	if !ok {
		tb.Fatalf("expected ActionRequestMsg, got %T", msg)
	}
	if action.Mode != wantMode || action.Action != wantAction {
		tb.Fatalf("unexpected action request: %#v", action)
	}
}
