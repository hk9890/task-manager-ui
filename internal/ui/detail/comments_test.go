package detail

import (
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/revier"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// commentsState is an issue with three comments at 100x30, with the keys the
// detail mode passes for the buttons. OrderedComments puts carol first.
func commentsState(t *testing.T) State {
	t.Helper()
	return State{
		SelectionID: "tm-77",
		Detail: domain.IssueDetail{
			Summary:     domain.IssueSummary{ID: "tm-77", Title: "Comments heavy issue", Status: "open", Type: "bug"},
			Description: "Short description",
			Comments: []domain.IssueComment{
				{ID: "c-1", Author: "alice", Body: "First chronologically", CreatedAt: mustTime(t, "2026-04-05T10:00:00Z")},
				{ID: "c-2", Author: "bob", Body: "Second line one\n\nSecond line two", CreatedAt: mustTime(t, "2026-04-05T12:00:00Z")},
				{ID: "c-3", Author: "carol", Body: "Third chronologically", CreatedAt: mustTime(t, "2026-04-05T14:00:00Z")},
			},
		},
		ButtonKeys: ButtonKeys{EditIssue: "alt+u", AddComment: "alt+a", EditComment: "enter", DeleteComment: "alt+d", LaunchAgent: "alt+i", FocusAgent: "alt+o"},
		Width:      100,
		Height:     30,
	}
}

func TestRenderSelectedCommentGolden(t *testing.T) {
	t.Parallel()

	state := commentsState(t)
	state.FocusPane = FocusPaneComments
	state.CommentIndex = 1
	testui.AssertMatchesGolden(t, []byte(Render(state)), "comments_selected_w100.golden")
}

func TestRenderNoCommentsGolden(t *testing.T) {
	t.Parallel()

	state := commentsState(t)
	state.Detail.Comments = nil
	testui.AssertMatchesGolden(t, []byte(Render(state)), "comments_none_w100.golden")
}

// TestAQuotedCommentIsMarkdownNotALog: a comment that quotes three lines or
// more is drawn as a quote; a log with prompts inside its lines is a log.
func TestAQuotedCommentIsMarkdownNotALog(t *testing.T) {
	t.Parallel()

	if isLogLikeComment("> first quoted line\n> second\n>\n> third\n\nWhat I think of it.") {
		t.Error("a quote of three lines and more is taken for a log")
	}
	if !isLogLikeComment("PS C:\\> dir\nPS C:\\> cd src\nPS C:\\> go test ./...") {
		t.Error("a log of prompts is no longer taken for a log")
	}
}

// A state that does not focus the comments draws no selected comment, whatever
// CommentIndex holds: the zero State must not mark comment 0.
func TestCommentsWithoutTheFocusDrawNoBarAndNoCommentButtons(t *testing.T) {
	t.Parallel()

	bar, _ := styles.SelectionPrefix(true, false)
	for _, focus := range []FocusPane{FocusPaneContent, FocusPaneDependencies} {
		state := commentsState(t)
		state.FocusPane = focus
		view := testui.Downsample(Render(state))
		for _, absent := range []string{bar + "[", "edit enter", "delete alt+d"} {
			if strings.Contains(view, absent) {
				t.Errorf("focus %d draws %q:\n%s", focus, absent, view)
			}
		}
		for _, present := range []string{"edit alt+u", "add alt+a", "[1/3] carol"} {
			if !strings.Contains(view, present) {
				t.Errorf("focus %d does not draw %q:\n%s", focus, present, view)
			}
		}
	}
}

// The selected comment has the bar on each of its lines, a blank one included,
// and no other comment has it. The line count does not follow the cursor.
func TestSelectedCommentHasTheBarOnEveryLine(t *testing.T) {
	t.Parallel()

	bar, _ := styles.SelectionPrefix(true, false)
	state := commentsState(t)
	state.FocusPane = FocusPaneComments
	unselected := renderContentPaneLines(state.contentPane(100, 30), 98, 28)

	for index := range state.Detail.Comments {
		state.CommentIndex = index
		content := renderContentPaneLines(state.contentPane(100, 30), 98, 28)
		if len(content.lines) != len(unselected.lines) {
			t.Fatalf("comment %d selected: %d lines, want the %d of every selection", index, len(content.lines), len(unselected.lines))
		}
		for line, owner := range content.comment {
			if got := strings.HasPrefix(testui.Downsample(content.lines[line]), bar); got != (owner == index) {
				t.Errorf("comment %d selected: line %d of comment %d has the bar: %v\n%q", index, line, owner, got, content.lines[line])
			}
		}
	}

	state.CommentIndex = 1
	first, last := CommentLineRange(state, 1)
	if last-first != 3 {
		t.Errorf("CommentLineRange of a header and three body lines = %d to %d", first, last)
	}
}

// Two comments are separated by a blank line, a rule in the border colour and
// a blank line, and the author line is bold.
func TestCommentsAreSeparatedByARuleAndTheAuthorLineIsBold(t *testing.T) {
	testui.ForceTrueColor(t)

	state := commentsState(t)
	content := renderContentPaneLines(state.contentPane(100, 30), 98, 28)

	rule := lipgloss.NewStyle().Foreground(styles.BorderDefaultColor).Render(strings.Repeat("─", 98))
	rules := 0
	for line, text := range content.lines {
		if line <= content.heading || text != rule {
			continue
		}
		rules++
		if content.lines[line-1] != "" || content.lines[line+1] != "" || content.comment[line] != -1 {
			t.Errorf("the rule on line %d does not stand between two blank lines of no comment", line)
		}
	}
	if rules != 2 {
		t.Errorf("three comments are separated by %d rules, want 2", rules)
	}

	author := lipgloss.NewStyle().Bold(true).Render("[1/3] carol · 2026-04-05 14:00")
	if !strings.Contains(strings.Join(content.lines, "\n"), author) {
		t.Errorf("the author line is not bold:\n%q", content.lines[content.heading+1])
	}
}

// Each button is found where Render drew it, in both layouts, and a comment
// line reports its comment.
func TestHitTestFindsTheButtonsAndTheCommentsWhereRenderDrewThem(t *testing.T) {
	t.Parallel()

	for _, width := range []int{160, 100} {
		state := commentsState(t)
		state.Width, state.Height = width, 50
		state.FocusPane = FocusPaneComments
		state.CommentIndex = 1
		state.Agents = []revier.Agent{{Panel: "3", Status: revier.StatusRunning, Session: "s-3"}}
		view := Render(state)

		for text, want := range map[string]Hit{
			"agent alt+i":  {Pane: FocusPaneContent, Button: ButtonLaunchAgent},
			"open alt+o":   {Pane: FocusPaneContent, Button: ButtonFocusAgent},
			"edit alt+u":   {Pane: FocusPaneContent, Button: ButtonEdit},
			"add alt+a":    {Pane: FocusPaneContent, Button: ButtonAddComment},
			"edit enter":   {Pane: FocusPaneComments, Comment: 1, Button: ButtonEditComment},
			"delete alt+d": {Pane: FocusPaneComments, Comment: 1, Button: ButtonDeleteComment},
		} {
			x, y := testui.FindCell(t, view, text)
			// The cell in front of the label and the last cell of the key
			// belong to the button too.
			for _, column := range []int{x - 1, x, x + len(text)} {
				if hit, ok := HitTest(state, column, y); !ok || hit != want {
					t.Errorf("width %d: HitTest on %q at column %d = %+v, %v; want %+v", width, text, column, hit, ok, want)
				}
			}
			if hit, _ := HitTest(state, x-2, y); hit.Button != ButtonNone {
				t.Errorf("width %d: the cell two in front of %q is the button %d", width, text, hit.Button)
			}
		}

		for text, want := range map[string]int{
			"[1/3] carol":           0,
			"Third chronologically": 0,
			"Second line two":       1,
			"[3/3] alice":           2,
		} {
			x, y := testui.FindCell(t, view, text)
			if hit, ok := HitTest(state, x, y); !ok || hit.Pane != FocusPaneComments || hit.Comment != want || hit.Button != ButtonNone {
				t.Errorf("width %d: HitTest on %q = %+v, %v; want comment %d", width, text, hit, ok, want)
			}
		}

		// The heading and the lines between two comments are no comment.
		x, y := testui.FindCell(t, view, "Comments (3")
		if hit, _ := HitTest(state, x, y); hit != (Hit{Pane: FocusPaneContent}) {
			t.Errorf("width %d: HitTest on the heading = %+v", width, hit)
		}
		_, y = testui.FindCell(t, view, "[2/3] bob")
		if hit, _ := HitTest(state, x, y-1); hit != (Hit{Pane: FocusPaneContent}) {
			t.Errorf("width %d: HitTest on the blank line over a comment = %+v", width, hit)
		}
	}
}

// A scrolled pane reports the comment under the pointer and nothing on its
// two indicator rows, and a skeleton pane has no button.
func TestHitTestFollowsTheScrolledContentWindow(t *testing.T) {
	t.Parallel()

	state := commentsState(t)
	state.Height = 16
	state.ContentScrollOffset = MaxScrollOffsets(state).Content
	view := Render(state)

	x, y := testui.FindCell(t, view, "[3/3] alice")
	if hit, ok := HitTest(state, x, y); !ok || hit.Pane != FocusPaneComments || hit.Comment != 2 {
		t.Errorf("HitTest on the last comment of a scrolled pane = %+v, %v", hit, ok)
	}
	x, y = testui.FindCell(t, view, "earlier)")
	if hit, _ := HitTest(state, x, y); hit != (Hit{Pane: FocusPaneContent}) {
		t.Errorf("HitTest on the indicator row = %+v, want the pane alone", hit)
	}

	state = commentsState(t)
	state.Skeleton = true
	if strings.Contains(Render(state), "edit") {
		t.Errorf("a skeleton pane draws a button:\n%s", Render(state))
	}
	for column := 0; column < state.Width; column++ {
		if hit, _ := HitTest(state, column, 1); hit.Button != ButtonNone {
			t.Fatalf("a skeleton pane reports the button %d at column %d", hit.Button, column)
		}
	}
}

// The edit button stands on the ID line and ends in the column the add button
// ends in. The border line holds no button.
func TestTheEditButtonEndsInTheColumnOfTheAddButton(t *testing.T) {
	t.Parallel()

	for _, width := range []int{160, 116, 100} {
		state := commentsState(t)
		state.Width = width
		view := Render(state)

		editX, editY := testui.FindCell(t, view, "edit alt+u")
		idX, idY := testui.FindCell(t, view, "tm-77")
		addX, _ := testui.FindCell(t, view, "add alt+a")
		if editY != idY || editX < idX {
			t.Errorf("width %d: edit at (%d,%d) is not at the right of the ID at (%d,%d)", width, editX, editY, idX, idY)
		}
		if editX+len("edit alt+u") != addX+len("add alt+a") {
			t.Errorf("width %d: edit ends in column %d, add in column %d", width, editX+len("edit alt+u"), addX+len("add alt+a"))
		}
		_, borderY := testui.FindCell(t, view, "Content ─")
		if border := strings.Split(view, "\n")[borderY]; strings.Contains(border, "edit") {
			t.Errorf("width %d: the border line holds a button: %s", width, border)
		}
	}
}

// An ID line too long for the button keeps the ID whole and draws no button.
func TestAnIDLineWithoutRoomDrawsNoEditButton(t *testing.T) {
	t.Parallel()

	pane := ContentPaneState{Detail: commentsState(t).Detail}
	pane.Detail.Summary.ID = "tm-a-long-issue-id"
	content := renderContentPaneLines(pane, 30, 20)
	if !strings.Contains(content.lines[0], "tm-a-long-issue-id") || strings.Contains(content.lines[0], "edit") || content.idButtons != nil {
		t.Errorf("the ID line is %q with the button at %+v, want the whole ID and no button", content.lines[0], content.idButtons)
	}
}

// The open button stands left of agent and edit only while an agent is
// linked to the issue. agent and edit keep their cells when it comes or goes,
// and edit still ends in the column of add.
func TestTheOpenButtonIsDrawnOnlyWithALinkedAgent(t *testing.T) {
	t.Parallel()

	state := commentsState(t)
	view := Render(state)
	if plain := testui.Downsample(view); strings.Contains(plain, "open alt+o") || !strings.Contains(plain, "agent alt+i  ·  edit alt+u") {
		t.Errorf("without a linked agent the ID line is not agent and edit:\n%s", plain)
	}
	agentX, agentY := testui.FindCell(t, view, "agent alt+i")
	editX, _ := testui.FindCell(t, view, "edit alt+u")

	state.Agents = []revier.Agent{{Panel: "3", Status: revier.StatusIdle, Session: "s-3"}}
	view = Render(state)
	if !strings.Contains(testui.Downsample(view), "open alt+o  ·  agent alt+i  ·  edit alt+u") {
		t.Errorf("with a linked agent the ID line is not open, agent and edit:\n%s", testui.Downsample(view))
	}
	if x, y := testui.FindCell(t, view, "agent alt+i"); x != agentX || y != agentY {
		t.Errorf("agent moved from (%d,%d) to (%d,%d) when open came", agentX, agentY, x, y)
	}
	if x, _ := testui.FindCell(t, view, "edit alt+u"); x != editX {
		t.Errorf("edit moved from column %d to %d when open came", editX, x)
	}
	addX, _ := testui.FindCell(t, view, "add alt+a")
	if editX+len("edit alt+u") != addX+len("add alt+a") {
		t.Errorf("edit ends in column %d, add in column %d", editX+len("edit alt+u"), addX+len("add alt+a"))
	}
}

// An ID line without room for every button keeps open and edit while an agent
// is linked, and edit alone without one, so agent never takes the cells open
// held. Narrower still, it draws edit alone.
func TestAnIDLineWithoutRoomForEveryButtonKeepsOpenAndEdit(t *testing.T) {
	t.Parallel()

	pane := commentsState(t).contentPane(0, 0)
	for _, tc := range []struct {
		width  int
		agents []revier.Agent
		want   []Button
	}{
		// 50 cells: the Content pane of the three-pane layout at 120 columns.
		{50, []revier.Agent{{Panel: "3", Status: revier.StatusIdle, Session: "s-3"}}, []Button{ButtonFocusAgent, ButtonEdit}},
		{50, nil, []Button{ButtonEdit}},
		{32, []revier.Agent{{Panel: "3", Status: revier.StatusIdle, Session: "s-3"}}, []Button{ButtonEdit}},
	} {
		pane.Agents = tc.agents
		content := renderContentPaneLines(pane, tc.width, 20)
		var got []Button
		for _, placed := range content.idButtons {
			got = append(got, placed.button)
		}
		line := testui.Downsample(content.lines[0])
		if !strings.Contains(line, "tm-77") || !strings.HasSuffix(line, "edit alt+u ") || strings.Contains(line, "agent") || !slices.Equal(got, tc.want) {
			t.Errorf("width %d, %d agents: the ID line is %q with %v, want the ID and %v", tc.width, len(tc.agents), line, got, tc.want)
		}
	}
}

// The button under the pointer takes the hover background and a bold label,
// as a menu-bar button does.
func TestTheButtonUnderThePointerIsLit(t *testing.T) {
	testui.ForceTrueColor(t)

	state := commentsState(t)
	state.FocusPane = FocusPaneComments
	state.Agents = []revier.Agent{{Panel: "3", Status: revier.StatusRunning, Session: "s-3"}}
	idle := Render(state)

	for button, drawn := range map[Button][2]string{
		ButtonLaunchAgent:   {"agent", "alt+i"},
		ButtonFocusAgent:    {"open", "alt+o"},
		ButtonEdit:          {"edit", "alt+u"},
		ButtonAddComment:    {"add", "alt+a"},
		ButtonEditComment:   {"edit", "enter"},
		ButtonDeleteComment: {"delete", "alt+d"},
	} {
		lit := paneButton(drawn[0], drawn[1], true)
		if strings.Contains(idle, lit) {
			t.Fatalf("button %d is lit without a pointer", button)
		}
		state.Hover = &Hit{Button: button}
		if view := Render(state); !strings.Contains(view, lit) {
			t.Errorf("button %d under the pointer is not drawn as %q", button, lit)
		}
	}

	label := lipgloss.NewStyle().Foreground(styles.ShellActionColor).Background(styles.ShellHoverBgColor).Bold(true).Render(" add ")
	if !strings.HasPrefix(paneButton("add", "alt+a", true), label) {
		t.Errorf("a lit button does not start with the bold label on the hover background: %q", paneButton("add", "alt+a", true))
	}
}

// A line with no room for the text draws no button and reports no cells.
func TestPlaceButtonsDropsTheButtonsWhenTheTextHasNoRoom(t *testing.T) {
	t.Parallel()

	button := paneButton("add", "alt+a", false)
	width := buttonTextFloor + lipgloss.Width(button)

	line, placed := placeButtons("Comments (0)", width, button)
	if len(placed) != 1 || lipgloss.Width(line) != width || !strings.HasSuffix(testui.Downsample(line), "add alt+a ") {
		t.Errorf("at width %d: %q with cells %v", width, line, placed)
	}

	line, placed = placeButtons("Comments (0)", width-1, button)
	if placed != nil || line != "Comments (0)" {
		t.Errorf("at width %d: %q with cells %v; want the text alone", width-1, line, placed)
	}
}
