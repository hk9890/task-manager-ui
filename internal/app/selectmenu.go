package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/displaytext"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/ui/overlay"
	"github.com/hk9890/task-manager-ui/internal/ui/selectmenu"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// selectionMenu is the menu a finished drag opens beside the release cell.
// The release has already sent the text to the clipboard; the menu offers
// what else to do with it. It stands over the screen the drag froze, box
// included, and holds every key and mouse event until it closes: each would
// change the screen it stands over.
type selectionMenu struct {
	text     string
	backdrop string
	at       screenCell
	// screenWidth is the width the menu is cut to.
	screenWidth int
	entries     []selectionAction
	cursor      int
	pointer     *screenCell
}

type selectionAction struct {
	entry selectmenu.Entry
	run   func(m *Model, text string) tea.Cmd
}

// openSelectionMenu opens the menu for a selection the release just sent.
func (m *Model) openSelectionMenu(selected textSelection, text string, release screenCell) {
	menu := &selectionMenu{text: text, backdrop: selected.view(), entries: m.selectionActions()}
	for menu.cursor < len(menu.entries)-1 && menu.entries[menu.cursor].entry.Disabled {
		menu.cursor++
	}
	menu.screenWidth = m.width
	width, height := selectmenu.Size(menu.state())
	menu.at.x, menu.at.y = selectmenu.Place(release.x, release.y, width, height, m.width, m.height)
	m.selMenu = menu
}

// selectionActions are the menu's entries. Quoting and searching leave the
// surface on screen, so they are off where that would drop work in progress
// or where there is no store to act on.
//
// The quote goes to the issue its label names: a refresh under the menu can
// move the selection before the entry is chosen.
func (m *Model) selectionActions() []selectionAction {
	onStore := !m.showHelp && !m.showActionModal && m.storeOpen &&
		m.active != mode.StorePicker && m.active != mode.Config && m.active != mode.IssueEdit
	quote := "Quote in a comment"
	var issue domain.IssueSummary
	selection := m.currentSelection()
	canQuote := onStore && selection != nil && selection.Issue.ID != ""
	if canQuote {
		issue = selection.Issue
		quote = "Quote in a comment on " + displaytext.OneLine(issue.ID)
	}
	return []selectionAction{
		{entry: selectmenu.Entry{Label: quote, Disabled: !canQuote}, run: func(m *Model, text string) tea.Cmd {
			m.quoteInComment(issue, text)
			return nil
		}},
		{entry: selectmenu.Entry{Label: "Search the store", Disabled: !onStore}, run: (*Model).searchSelection},
		{
			entry: selectmenu.Entry{Label: "Close", Key: m.keys.DisplayPrimary(config.ShellContext, config.ShellActionEscape)},
			run:   func(*Model, string) tea.Cmd { return nil },
		},
	}
}

func (s *selectionMenu) state() selectmenu.State {
	state := selectmenu.State{
		Title:  fmt.Sprintf("Sent %d characters to the clipboard", sentCount(s.text)),
		Hint:   "shift+drag if not copied",
		Width:  s.screenWidth,
		Cursor: s.cursor,
		Hover:  -1,
	}
	for _, action := range s.entries {
		state.Entries = append(state.Entries, action.entry)
	}
	if s.pointer != nil {
		if entry, _ := selectmenu.HitTest(state, s.pointer.x-s.at.x, s.pointer.y-s.at.y); entry >= 0 {
			state.Hover = entry
		}
	}
	return state
}

func (s *selectionMenu) view(width, height int) string {
	menu := selectmenu.Render(s.state())
	return overlay.Place(overlay.Config{Width: width, Height: height, Position: overlay.At, X: s.at.x, Y: s.at.y}, menu, s.backdrop)
}

// selectionMenuKey works the menu by key: up and down move the cursor over
// the entries that are on, enter runs the one under it, Escape closes the
// menu and quit quits. Every other key is dropped.
func (m Model) selectionMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case m.keys.Match(config.ShellContext, config.ShellActionQuit, msg):
		return m, tea.Quit
	case m.keys.Match(config.ShellContext, config.ShellActionEscape, msg):
		m.selMenu = nil
		return m, nil
	}
	switch {
	case mode.PlainKey(msg, tea.KeyUp):
		m.selMenu.move(-1)
	case mode.PlainKey(msg, tea.KeyDown):
		m.selMenu.move(1)
	case mode.PlainKey(msg, tea.KeyEnter):
		return m.runSelectionAction(m.selMenu.cursor)
	}
	return m, nil
}

func (s *selectionMenu) move(step int) {
	for next := s.cursor + step; next >= 0 && next < len(s.entries); next += step {
		if !s.entries[next].entry.Disabled {
			s.cursor = next
			return
		}
	}
}

// selectionMenuMouse lights the entry under the pointer and runs the one a
// left click lands on. A click outside the menu closes it and does nothing
// else, so the row under it is not selected by surprise.
func (m Model) selectionMenuMouse(msg pointer) (tea.Model, tea.Cmd) {
	menu := m.selMenu
	at := screenCell{x: msg.X, y: msg.Y}
	switch {
	case msg.act == pointerMotion:
		menu.pointer = &at
	case msg.act == pointerPress && msg.Button == tea.MouseLeft:
		entry, inside := selectmenu.HitTest(menu.state(), at.x-menu.at.x, at.y-menu.at.y)
		switch {
		case !inside:
			m.selMenu = nil
		case entry >= 0:
			return m.runSelectionAction(entry)
		}
	}
	return m, nil
}

func (m Model) runSelectionAction(entry int) (tea.Model, tea.Cmd) {
	action := m.selMenu.entries[entry]
	if action.entry.Disabled {
		return m, nil
	}
	text := m.selMenu.text
	m.selMenu = nil
	return m, action.run(&m, text)
}

// quoteInComment opens a new comment on issue with the text in it as a
// Markdown quote, and the cursor on the line after it.
func (m *Model) quoteInComment(issue domain.IssueSummary, text string) {
	lines := quotable(text)
	for i, line := range lines {
		lines[i] = strings.TrimRight("> "+line, " ")
	}
	m.openCommentOn(issue, strings.Join(lines, "\n")+"\n\n")
}

// quotable is the text of a selection without the screen's chrome around it:
// the columns at the start that hold a border, the selection gutter or a
// space on every line, the borders and spaces at the end of each line, and
// the lines left empty at the top and the bottom. A line keeps the indent it
// has over the others. The clipboard gets the box as it is drawn.
func quotable(text string) []string {
	lines := strings.Split(text, "\n")
	cells := make([][]rune, len(lines))
	for i, line := range lines {
		cells[i] = []rune(strings.TrimRightFunc(line, isBorder))
	}
	left := 0
	for chromeColumn(cells, left) {
		left++
	}
	var out []string
	for _, line := range cells {
		out = append(out, string(line[min(left, len(line)):]))
	}
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// chromeColumn reports whether column col is chrome on every line that
// reaches it, and some line does. Each cell before col is chrome, one cell
// wide, so col counts runes and cells alike.
func chromeColumn(lines [][]rune, col int) bool {
	reached := false
	for _, line := range lines {
		if col >= len(line) {
			continue
		}
		if !isBorder(line[col]) && string(line[col]) != styles.Glyphs.Cursor {
			return false
		}
		reached = true
	}
	return reached
}

// isBorder reports a space, or a box-drawing or block glyph: what the panes
// draw their borders and the selection bar with.
func isBorder(r rune) bool {
	return r == ' ' || (r >= '─' && r <= '▟')
}

// searchSelection opens the store search with the text as its query, on one
// line. The query is set before the search opens, so the one search it runs
// is for the text: the first open's Init and the open's auto refresh both
// leave a search in flight alone.
func (m *Model) searchSelection(text string) tea.Cmd {
	search := m.scoped(m.search.SetQuery(strings.Join(strings.Fields(strings.Join(quotable(text), " ")), " ")))
	return batchCmds(search, m.openSearch())
}

// sentCount is the characters of a selection, its line breaks not counted.
func sentCount(text string) int {
	return len([]rune(strings.ReplaceAll(text, "\n", "")))
}
