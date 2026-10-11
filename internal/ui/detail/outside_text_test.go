package detail

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
)

// assertWholeFrame fails when view is not a frame of height rows, each exactly
// width cells wide and free of control characters. A row that is wider runs
// past the terminal edge and moves every row below it down a line.
func assertWholeFrame(t *testing.T, view string, width, height int) {
	t.Helper()

	lines := strings.Split(view, "\n")
	if len(lines) != height {
		t.Errorf("the frame has %d rows, want %d", len(lines), height)
	}
	for _, line := range lines {
		if got := lipgloss.Width(line); got != width {
			t.Errorf("a row is %d cells wide, want %d: %q", got, width, textutil.StripANSI(line))
		}
		for _, r := range textutil.StripANSI(line) {
			if r < ' ' {
				t.Errorf("control character %q in a row: %q", r, textutil.StripANSI(line))
			}
		}
	}
}

// TestATabInABodyKeepsTheFrameWhole: a TAB measures as no cell and the
// terminal draws it to the next tab stop, so a body line with one was drawn
// wider than its pane.
func TestATabInABodyKeepsTheFrameWhole(t *testing.T) {
	t.Parallel()

	const body = "## Problem\n\n    $ gh release list\n    GNOME Window Control v12\tLatest\tv12\t2026-10-02T15:34:07Z\n\n" +
		"```go\nfunc main() {\n\tif true {\n\t\tfmt.Println(\"tab\")\n\t}\n}\n```\n\n- [ ] a criterion with a\ttab\n\n> a quote with a\ttab\n\nA paragraph with a\ttab."

	for _, width := range []int{100, 160, 200} {
		view := Render(State{
			SelectionID: "tm-1",
			Detail: domain.IssueDetail{
				Summary:     domain.IssueSummary{ID: "tm-1", Title: "Tabs", Status: "open", Type: "task", Priority: 2},
				Description: body,
				Notes:       body,
				Comments: []domain.IssueComment{
					{ID: "c-1", Author: "alice", Body: body, CreatedAt: mustTime(t, "2026-04-05T10:00:00Z")},
					{ID: "c-2", Author: "bob", Body: "## Note\n\nwith a\ttab", CreatedAt: mustTime(t, "2026-04-05T11:00:00Z")},
				},
			},
			Width:  width,
			Height: 60,
		})
		assertWholeFrame(t, view, width, 60)
	}
}

// rightToLeftOverride is built from its code point: written into a string
// literal it is invisible, and it reorders the line it stands in.
var rightToLeftOverride = string(rune(0x202e))

// TestAControlCharacterInAnIssueFieldKeepsTheFrameWhole: every field of an
// issue is text from outside. A line feed in one added a row to the frame, and
// an escape sequence would run in the terminal.
func TestAControlCharacterInAnIssueFieldKeepsTheFrameWhole(t *testing.T) {
	t.Parallel()

	for _, dirt := range []string{"\t", "\n", "\r", "\x1b[2J", rightToLeftOverride} {
		dirty := "a" + dirt + "b"
		reference := []domain.IssueReference{{ID: "tm-" + dirty, Title: dirty, Type: "task", Priority: 2, Status: "open"}}
		detail := domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID: "tm-" + dirty, Title: dirty, Status: dirty, Type: "task", Priority: 2,
				Assignee: dirty, Labels: []string{dirty},
			},
			Creator:     dirty,
			CloseReason: dirty,
			ClosedAt:    mustTime(t, "2026-04-05T12:00:00Z"),
			Description: "Description",
			BlockedBy:   reference,
			Blocks:      reference,
			Related:     reference,
			Children:    reference,
			Comments:    []domain.IssueComment{{ID: "c-1", Author: dirty, Body: "text", CreatedAt: mustTime(t, "2026-04-05T10:00:00Z")}},
		}

		for _, width := range []int{80, 100, 160} {
			view := Render(State{SelectionID: detail.Summary.ID, Detail: detail, Width: width, Height: 60})
			if dirt != "\n" && strings.Contains(view, dirt) {
				t.Errorf("%q from an issue field reaches the terminal at width %d", dirt, width)
			}
			if width >= 100 {
				assertWholeFrame(t, view, width, 60)
			}
		}
	}
}

// TestALogLikeCommentWrapsItsLongLines: a log-like comment was cut at the pane
// edge with an ellipsis, and a pane scrolls down only, so the end of each long
// line could not be read anywhere.
func TestALogLikeCommentWrapsItsLongLines(t *testing.T) {
	t.Parallel()

	const width = 60
	words := strings.TrimSpace(strings.Repeat("word ", 24))
	body := strings.Join([]string{words + " first-end", words + " second-end", words + " third-end"}, "\n")

	lines, _, _ := renderComments([]domain.IssueComment{
		{ID: "c-1", Author: "alice", Body: body, CreatedAt: mustTime(t, "2026-04-05T11:00:00Z")},
	}, width, -1)

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "├─ output") {
		t.Fatalf("expected the framed log-like presentation, got:\n%s", joined)
	}
	for _, want := range []string{"first-end", "second-end", "third-end"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the end of a long line is lost: %q is not in\n%s", want, joined)
		}
	}
	for _, line := range lines {
		if got := lipgloss.Width(line); got > width {
			t.Errorf("a comment line is %d cells wide, want at most %d: %q", got, width, line)
		}
		if strings.HasSuffix(line, "…") {
			t.Errorf("a comment line is cut: %q", line)
		}
	}
}
