package app

import (
	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/ui/helpscreen"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
	"github.com/hk9890/task-manager-ui/internal/version"
)

// The help screen: every key the app answers to, in one place. The legend
// names only the keys of the surface on screen and drops the rest on a narrow
// terminal. The screen is drawn from the resolved bindings on every frame, so
// it holds no text of an older theme and no key a config rebound.

// helpWheelLines is how far one wheel notch scrolls the help screen, where a
// page is that long.
const helpWheelLines = 3

// helpSections is the screen's content, in the order a new operator needs it.
// A key is read from the binding that runs it, and every action of every
// context has an entry (TestHelpNamesEveryBoundAction). The keys spelled out
// here are the ones no binding names: the query's own, the scope key of the
// store search, the pane keys and enter in Detail, y and n in a dialog, the
// keys that step a value on the configuration screen, the keys that move in
// the issue editor, and the mouse.
func helpSections(keys config.ResolvedKeyBindings) []helpscreen.Section {
	shell := func(action string) string { return keys.DisplayLabel(config.ShellContext, action) }
	board := func(action string) string { return keys.DisplayLabel(config.BoardContext, action) }
	detail := func(action string) string { return keys.DisplayLabel(config.DetailContext, action) }
	dialog := func(action string) string { return keys.DisplayLabel(config.ModalContext, action) }
	pair := func(first, second string) string { return first + " / " + second }

	return []helpscreen.Section{
		{Title: "Moving and opening", Entries: []helpscreen.Entry{
			{Key: pair(board(config.BoardActionMoveLeft), board(config.BoardActionMoveRight)), Desc: "the column to the left / right, on the Board"},
			{Key: pair(board(config.BoardActionMoveUp), board(config.BoardActionMoveDown)), Desc: "the row above / below"},
			{Key: pair(board(config.BoardActionPageUp), board(config.BoardActionPageDown)), Desc: "a page up / down"},
			{Key: pair(board(config.BoardActionMoveHome), board(config.BoardActionMoveEnd)), Desc: "the first / last row"},
			{Key: board(config.BoardActionOpenDetail), Desc: "open the selected issue in Detail"},
			{Key: shell(config.ShellActionEscape), Desc: "back to where the surface was opened from; hide a toast"},
		}},
		{Title: "Filter and search", Entries: []helpscreen.Entry{
			{Key: "type", Desc: "filter the rows of a tab by title or ID; every word must match"},
			{Key: "backspace", Desc: "delete a character of the query"},
			{Key: "ctrl+w", Desc: "delete a word"},
			{Key: "ctrl+u", Desc: "delete the query"},
			{Key: shell(config.ShellActionEscape), Desc: "clear the query"},
			{Key: shell(config.ShellActionOpenSearch), Desc: "search the store: titles, IDs and descriptions"},
			{Key: "ctrl+t", Desc: "in the search, switch between open issues and all of them"},
		}},
		{Title: "Tabs", Entries: []helpscreen.Entry{
			{Key: shell(config.ShellActionModeCycleNext), Desc: "next tab: Board, Docs"},
			{Key: shell(config.ShellActionModeCyclePrev), Desc: "previous tab"},
		}},
		{Title: "Issue", Entries: []helpscreen.Entry{
			{Key: shell(config.ShellActionCreateIssue), Desc: "create an issue, in the issue editor"},
			{Key: shell(config.ShellActionUpdateIssue), Desc: "edit the selected issue, its description included"},
			{Key: shell(config.ShellActionCommentIssue), Desc: "comment on the selected issue, in the issue editor"},
			{Key: shell(config.ShellActionCloseIssue), Desc: "close the selected issue: opens the editor on the close reason"},
		}},
		{Title: "Issue editor", Entries: []helpscreen.Entry{
			{Key: pair("tab", "shift+tab"), Desc: "the next / previous field or button"},
			{Key: pair("up", "down"), Desc: "the previous / next field; in the text, move the cursor, and leave the text from its first / last line"},
			{Key: pair("left", "right"), Desc: "the value before / after the one shown, on Status, Type and Priority; the other button"},
			{Key: "enter", Desc: "the next field, or press the focused button; in the text, a new line"},
			{Key: pair("pgup", "pgdown"), Desc: "in the text, a page up / down"},
			{Key: shell(config.ShellActionSaveEdit), Desc: "save"},
			{Key: shell(config.ShellActionEscape), Desc: "leave without a save"},
		}},
		{Title: "Detail and launchers", Entries: []helpscreen.Entry{
			{Key: pair(detail(config.DetailActionScrollUp), detail(config.DetailActionScrollDown)), Desc: "scroll a line up / down, on this screen too"},
			{Key: pair(detail(config.DetailActionPageUp), detail(config.DetailActionPageDown)), Desc: "scroll a page up / down"},
			{Key: pair(detail(config.DetailActionHome), detail(config.DetailActionEnd)), Desc: "to the top / the bottom"},
			{Key: pair("left", "right"), Desc: "the pane to the left / right; the comments are a stop after Content"},
			{Key: pair(detail(config.DetailActionScrollUp), detail(config.DetailActionScrollDown)), Desc: "on the comments, the comment above / below"},
			{Key: "enter", Desc: "in the Dependencies pane, open the selected issue"},
			{Key: "enter", Desc: "on a comment, edit it"},
			{Key: detail(config.DetailActionDeleteComment), Desc: "on a comment, delete it after a yes"},
			{Key: shell(config.ShellActionReloadDetail), Desc: "read the issue from the store again"},
			{Key: shell(config.ShellActionLaunchNvim), Desc: "launch nvim on the issue, in the background"},
			{Key: shell(config.ShellActionLaunchOpencode), Desc: "launch opencode on the issue, in the background"},
			{Key: shell(config.ShellActionLaunchShell), Desc: "launch the shell command on the issue, in the background"},
		}},
		{Title: "Menu bar and stores", Entries: []helpscreen.Entry{
			{Key: shell(config.ShellActionStorePicker), Desc: "list the task stores on this machine and open one"},
			{Key: board(config.BoardActionReload), Desc: "reload the tab, the search or the store list"},
			{Key: shell(config.ShellActionOpenConfig), Desc: "open the configuration screen: the theme and the glyph set"},
			{Key: shell(config.ShellActionHelp), Desc: "show and hide this screen"},
			{Key: shell(config.ShellActionQuit), Desc: "quit"},
		}},
		{Title: "Dialogs", Entries: []helpscreen.Entry{
			{Key: dialog(config.ModalActionNext), Desc: "the next field or button"},
			{Key: dialog(config.ModalActionPrev), Desc: "the previous field or button"},
			{Key: pair(dialog(config.ModalActionLeft), dialog(config.ModalActionRight)), Desc: "the button to the left / right"},
			{Key: dialog(config.ModalActionEnter), Desc: "press the focused button; in a field, go to the next field"},
			{Key: "y", Desc: "confirm, while a button has the focus"},
			{Key: "n", Desc: "cancel, while a button has the focus"},
			{Key: dialog(config.ModalActionEscape), Desc: "cancel the dialog"},
		}},
		{Title: "Configuration screen", Entries: []helpscreen.Entry{
			{Key: pair(board(config.BoardActionMoveUp), board(config.BoardActionMoveDown)), Desc: "the setting above / below"},
			{Key: pair("left", "right"), Desc: "the value before / after the one shown"},
			{Key: "enter", Desc: "the value after the one shown"},
			{Key: shell(config.ShellActionEscape), Desc: "back to where the screen was opened from"},
		}},
		{Title: "Mouse", Entries: []helpscreen.Entry{
			{Key: "click", Desc: "select a row or a comment, switch to a tab, focus a pane or a field of the issue editor"},
			{Key: "click", Desc: "press a button: on the menu bar, in a Detail pane, Save and Cancel in the issue editor"},
			{Key: "second click", Desc: "open the row"},
			{Key: "wheel", Desc: "move the selection, or scroll Detail text, the text of the issue editor and this screen"},
			{Key: "drag", Desc: "select a box of text; letting go sends it to the terminal clipboard and opens a menu"},
			{Key: "shift+drag", Desc: "select text with the terminal instead"},
		}},
		{Title: "Selection menu", Entries: []helpscreen.Entry{
			{Key: "up/down", Desc: "the entry above / below"},
			{Key: "enter", Desc: "run the entry; a click on it does the same"},
			{Key: shell(config.ShellActionEscape), Desc: "close the menu"},
			{Key: "click outside", Desc: "close the menu, and do nothing else"},
		}},
	}
}

// helpHints is the help screen's own legend: the shell footer is not drawn
// while it is up.
func helpHints(keys config.ResolvedKeyBindings) []styles.KeyHint {
	primary := keys.DisplayPrimary
	pair := func(first, second string) string {
		return primary(config.DetailContext, first) + "/" + primary(config.DetailContext, second)
	}
	return []styles.KeyHint{
		{Key: pair(config.DetailActionScrollDown, config.DetailActionScrollUp), Desc: "scroll"},
		{Key: primary(config.ShellContext, config.ShellActionEscape), Desc: "back"},
		{Key: pair(config.DetailActionPageUp, config.DetailActionPageDown), Desc: "page"},
		{Key: pair(config.DetailActionHome, config.DetailActionEnd), Desc: "bounds"},
	}
}

func (m Model) renderHelp() string {
	return helpscreen.Render(helpscreen.State{
		Sections: helpSections(m.keys),
		Offset:   m.helpOffset,
		Version:  version.Version,
		Legend:   styles.KeyLegend(helpHints(m.keys), styles.ScreenTextWidth(m.width)),
		Width:    m.width,
		Height:   m.height,
	})
}

// helpKey is every press on the help screen. The screen only reads, so a
// press scrolls it or leaves it, and the key that opened it closes it too:
// matching a literal there left Escape as the only way out for a config that
// rebinds toggle_help.
func (m *Model) helpKey(k tea.KeyPressMsg) {
	if m.keys.Match(config.ShellContext, config.ShellActionHelp, k) ||
		m.keys.Match(config.ShellContext, config.ShellActionEscape, k) {
		m.showHelp = false
		return
	}
	m.scrollHelp(k)
}

// scrollHelp scrolls the help screen on the keys that scroll a detail pane.
// The wheel scrolls it too, and the mouse only repeats what a key does
// (docs/DESIGN-GUIDE.md).
func (m *Model) scrollHelp(k tea.KeyPressMsg) {
	detailKey := func(action string) bool { return m.keys.Match(config.DetailContext, action, k) }
	page := helpscreen.PageRows(m.height)
	switch {
	case detailKey(config.DetailActionScrollUp):
		m.scrollHelpBy(-1)
	case detailKey(config.DetailActionScrollDown):
		m.scrollHelpBy(1)
	case detailKey(config.DetailActionPageUp):
		m.scrollHelpBy(-page)
	case detailKey(config.DetailActionPageDown):
		m.scrollHelpBy(page)
	case detailKey(config.DetailActionHome):
		m.helpOffset = 0
	case detailKey(config.DetailActionEnd):
		m.scrollHelpBy(m.helpLastOffset())
	}
}

// scrollHelpBy moves the help screen by lines, from the line it draws now: a
// resize can leave the stored offset past the last one.
func (m *Model) scrollHelpBy(lines int) {
	last := m.helpLastOffset()
	m.helpOffset = textutil.Clamp(min(m.helpOffset, last)+lines, 0, last)
}

func (m Model) helpLastOffset() int {
	return helpscreen.MaxOffset(helpSections(m.keys), m.width, m.height)
}
