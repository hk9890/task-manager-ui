package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/ui/fatalerror"
	"github.com/hk9890/task-manager-ui/internal/ui/loading"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
	"github.com/hk9890/task-manager-ui/internal/version"
)

// View renders the root shell.
func (m Model) View() string {
	// Suppress the very first render until the terminal has sent us its real
	// dimensions via WindowSizeMsg. Without this guard the TUI emits a short
	// frame (defaultViewportHeight lines) immediately on startup; when
	// WindowSizeMsg then arrives the renderer produces a taller frame but only
	// partially overwrites the first one, leaving stale column-top border rows
	// visible above the correct render.
	if !m.sizeKnown {
		return ""
	}

	// A drag selects text on the screen as it was when the drag began.
	if m.sel.active {
		return m.sel.view()
	}

	if m.fatalErrTitle != "" {
		return fatalerror.Render(fatalerror.State{
			Title:  m.fatalErrTitle,
			Body:   m.fatalErrBody,
			Width:  m.width,
			Height: m.height,
		})
	}

	view := m.renderSurface()

	// Every overlay is applied in one place, for whatever surface is underneath.
	// An overlay that renders for the shell and not for the picker is still live
	// and still holds the keyboard — invisible, and with no way out.
	if m.toast.Visible() {
		view = m.toast.Overlay(view, m.width, m.height)
	}
	if m.showActionModal {
		view = m.actionModal.Overlay(view)
	}
	if m.showHelp {
		view = m.help.Overlay(view)
	}

	return view
}

// renderSurface renders whatever surface is active, without overlays.
//
// The picker is not a tab and not a drill-in: it renders instead of the shell,
// so the header and the legend are absent while it is up and it draws its own
// legend (docs/DESIGN-GUIDE.md).
func (m Model) renderSurface() string {
	if m.active == mode.StorePicker {
		return firstLines(m.storePicker.View(m.spinnerFrame, styles.KeyLegend(storePickerHints(m.keys, m.storeOpen), m.width)), m.height)
	}

	// A renderer keeps a floor of rows however short the workspace is. The body
	// is cut to the workspace, so the header stays on row 0 and the legend on
	// the row handleMouse takes for it: Bubble Tea drops the top of a frame
	// taller than the terminal, and every click then lands a row off.
	header, footer := m.renderHeader(), m.renderFooter()
	body := firstLines(m.renderBody(), m.workspaceHeight(header, footer))
	return firstLines(lipgloss.JoinVertical(lipgloss.Left, header, body, footer), m.height)
}

// firstLines cuts s to its first count lines.
func firstLines(s string, count int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= count {
		return s
	}
	return strings.Join(lines[:max(0, count)], "\n")
}

// The header's geometry. renderHeader draws from it, and tabAt and barCells
// read a column back through it, so a click lands on what is drawn there.
const (
	// The three header lines: the menu bar, the rule, the view tabs.
	headerMenuRow = 0
	headerTabsRow = 2

	headerMenuStart   = 1
	headerSpinnerCols = 2
	headerTabPadding  = 1
	headerTabGap      = 1
	// headerBarGap is the least space between the last button and the version.
	headerBarGap = 2
	// storeLabelMax is the widest a store name is drawn on the menu bar, and
	// storeLabelFloor the narrowest it is cut to before a button is dropped.
	storeLabelMax   = 24
	storeLabelFloor = 8
	// barSeparator stands between two buttons, as it does between two key
	// hints on the legend.
	barSeparator = " · "
)

// tabLabels names each browse tab on the tab line.
var tabLabels = map[mode.ID]string{
	mode.Board: "Board",
	mode.Docs:  "Docs",
}

// barAction is one button on the menu bar: what the shell can do that is not
// about the row under the cursor. A button carries the key bound to its
// action — the bar is a second way to reach the same thing, never the only
// way.
type barAction struct {
	label func(Model) string
	key   func(Model) string
	run   func(*Model) tea.Cmd
	// name marks the label as the store's name: the one button that says
	// something about the session. It is drawn bold, and cut to the room the
	// bar has.
	name bool
}

func barLabel(label string) func(Model) string {
	return func(Model) string { return label }
}

func shellKey(action string) func(Model) string {
	return func(m Model) string { return m.keys.DisplayPrimary(config.ShellContext, action) }
}

// The store button comes first: which store is on screen is the first thing
// the header must say, and the first button is the last one a narrow bar drops.
var barActions = []barAction{
	{label: Model.storeLabel, key: shellKey(config.ShellActionStorePicker), run: (*Model).openStorePicker, name: true},
	{label: barLabel("search"), key: shellKey(config.ShellActionOpenSearch), run: (*Model).openSearch},
	{label: barLabel("reload"), key: Model.reloadKey, run: (*Model).reloadActiveSurface},
	{label: barLabel("help"), key: shellKey(config.ShellActionHelp), run: (*Model).openHelp},
	{label: barLabel("quit"), key: shellKey(config.ShellActionQuit), run: (*Model).quit},
}

// storeLabel is the store button's label: the name of the active store, so the
// button says which store is open and what a click on it changes.
func (m Model) storeLabel() string {
	name := strings.TrimSpace(m.services.StoreName)
	if name == "" {
		return "stores"
	}
	return textutil.TruncateString(name, storeLabelMax)
}

// reloadKey is the key that reloads the active surface: each surface binds its
// own.
func (m Model) reloadKey() string {
	if m.active == mode.Detail {
		return m.keys.DisplayPrimary(config.ShellContext, config.ShellActionReloadDetail)
	}
	return m.keys.DisplayPrimary(config.BoardContext, config.BoardActionReload)
}

// barCell is a button's place on the bar: its text, and the columns it covers.
type barCell struct {
	action barAction
	label  string
	key    string
	x0, x1 int
}

// headerTabsStart is the column the first tab starts at.
func headerTabsStart() int {
	return headerSpinnerCols
}

// headerTabsEnd is the column after the last tab.
func headerTabsEnd() int {
	end := headerTabsStart()
	for idx, id := range mode.BrowseModes {
		if idx > 0 {
			end += headerTabGap
		}
		end += lipgloss.Width(tabLabels[id]) + 2*headerTabPadding
	}
	return end
}

// barCells places the buttons from the left edge. A bar that does not fit
// first cuts the store's name down to storeLabelFloor, then drops buttons, the
// rightmost first. The last button left gives up its floor too, so the bar
// names the store at any width that holds a cell of it.
func (m Model) barCells() []barCell {
	for count := len(barActions); count > 0; count-- {
		floor := storeLabelFloor
		if count == 1 {
			floor = 1
		}
		if cells, ok := m.placeBar(barActions[:count], floor); ok {
			return cells
		}
	}
	return nil
}

// placeBar places actions on the bar, with the name cut to the room the other
// text leaves. It reports false when the actions do not fit with the name cut
// to floor cells.
func (m Model) placeBar(actions []barAction, floor int) ([]barCell, bool) {
	cells := make([]barCell, 0, len(actions))
	used := headerMenuStart + (len(actions)-1)*lipgloss.Width(barSeparator)
	for _, action := range actions {
		cell := barCell{action: action, label: action.label(m), key: action.key(m)}
		cells = append(cells, cell)
		used += lipgloss.Width(cell.text())
	}

	x := headerMenuStart
	for idx := range cells {
		cell := &cells[idx]
		if cell.action.name {
			full := lipgloss.Width(cell.label)
			room := full + m.width - used
			if room < min(floor, full) {
				return nil, false
			}
			cell.label = textutil.TruncateString(cell.label, min(room, full))
			used += lipgloss.Width(cell.label) - full
		}
		cell.x0 = x
		cell.x1 = x + lipgloss.Width(cell.text())
		x = cell.x1 + lipgloss.Width(barSeparator)
	}
	return cells, used <= m.width
}

func (c barCell) text() string {
	if c.key == "" {
		return c.label
	}
	return c.label + " " + c.key
}

func (c barCell) covers(x int) bool {
	return x >= c.x0 && x < c.x1
}

// headerSpinnerCell returns a fixed 2-cell string: the current spinner glyph
// followed by a space when any surface is loading, or two literal spaces when
// idle. Using a fixed-width cell keeps the tabs where tabAt looks for them.
func (m Model) headerSpinnerCell() string {
	style := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	if m.workInFlight() {
		return style.Render(loading.Glyph(m.spinnerFrame) + " ")
	}
	return style.Render("  ")
}

// renderHeader draws the three lines above the workspace: the menu bar, the
// rule under it, and the view tabs.
func (m Model) renderHeader() string {
	return m.renderMenuBar() + "\n" + m.renderRule() + "\n" + m.renderTabs()
}

// renderMenuBar draws the buttons from the left and the version flush right.
// The version is the first to go when the two do not fit.
func (m Model) renderMenuBar() string {
	label := lipgloss.NewStyle().Foreground(styles.ShellActionColor)
	muted := lipgloss.NewStyle().Foreground(styles.ShellFooterHelpColor)

	cells := m.barCells()
	bar := ""
	for idx, cell := range cells {
		lead := muted.Render(barSeparator)
		if idx == 0 {
			lead = strings.Repeat(" ", headerMenuStart)
		}
		labelStyle := label.Bold(cell.action.name)
		if m.barPointer != nil && cell.covers(*m.barPointer) {
			labelStyle = label.Foreground(styles.ShellTabHoverColor).Bold(true)
		}
		bar += lead + labelStyle.Render(cell.label)
		if cell.key != "" {
			bar += " " + muted.Render(cell.key)
		}
	}

	free := m.width - lipgloss.Width(bar) - lipgloss.Width(version.Version)
	if len(cells) == len(barActions) && free >= headerBarGap {
		bar += strings.Repeat(" ", free) + muted.Render(version.Version)
	}
	return bar
}

// renderRule is the line between the menu bar and the view tabs.
func (m Model) renderRule() string {
	return styles.Rule(m.width)
}

// renderTabs draws the view tabs. The menu bar names the store, the active tab
// the surface and the highlighted row the selection, so the line says nothing
// more.
func (m Model) renderTabs() string {
	tab := func(id mode.ID) string {
		base := lipgloss.NewStyle().Padding(0, headerTabPadding)
		switch {
		case m.active == id:
			base = base.Foreground(styles.ShellTabActiveTextColor).Background(styles.ShellTabActiveBgColor).Bold(true)
		case m.hoverTab == id:
			base = base.Foreground(styles.ShellTabHoverColor)
		default:
			base = base.Foreground(styles.ShellTabInactiveColor)
		}
		return base.Render(tabLabels[id])
	}

	parts := []string{m.headerSpinnerCell()}
	for idx, id := range mode.BrowseModes {
		if idx > 0 {
			parts = append(parts, strings.Repeat(" ", headerTabGap))
		}
		parts = append(parts, tab(id))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// renderBody renders the active surface. Like every other renderer here it is
// pure: sizing runs from the WindowSizeMsg handler, not from View().
func (m Model) renderBody() string {
	skeletonPhase := loading.SkeletonPhase(m.spinnerFrame)

	if m.active == mode.Detail {
		return m.detail.View(m.detailViewportWidth(), m.detailViewportHeight(), false, skeletonPhase)
	}
	if tab := m.browseController(m.active); tab != nil {
		return tab.View(skeletonPhase)
	}
	// Board is the shell's home tab, so an unknown active mode renders it
	// rather than an empty frame.
	return m.board.View(skeletonPhase)
}

func (m Model) detailViewportHeight() int {
	_, workspaceHeight := m.workspaceSize()
	return workspaceHeight
}

func (m Model) detailViewportWidth() int {
	workspaceWidth, _ := m.workspaceSize()
	return workspaceWidth
}

func (m Model) workspaceSize() (int, int) {
	return max(1, m.width), m.workspaceHeight(m.renderHeader(), m.renderFooter())
}

// workspaceHeight is the rows the terminal has left between a rendered header
// and footer.
func (m Model) workspaceHeight(header, footer string) int {
	return max(1, m.height-lipgloss.Height(header)-lipgloss.Height(footer))
}

func (m Model) applyWorkspaceSizeToBrowseModes() {
	workspaceWidth, workspaceHeight := m.workspaceSize()
	for _, entry := range m.browseTabs() {
		entry.Tab.SetSize(workspaceWidth, workspaceHeight)
	}
}

// renderFooter draws the key legend. It is one line whatever the width: the
// workspace height is computed from it, so a hint that does not fit is dropped
// rather than wrapped.
func (m Model) renderFooter() string {
	if !m.services.Config.UI.ShowModeSwitcherHelp {
		return ""
	}

	hints := footerHints(m.active, m.keys)
	// Detail is where the launch keys work, so it is where their being off is
	// said, before the operator presses one. It leads the legend so that it is
	// the last hint a narrow terminal drops.
	if m.active == mode.Detail && m.projectRootMissing {
		hints = append([]styles.KeyHint{{Desc: "launchers off: project path missing"}}, hints...)
	}
	return styles.KeyLegend(hints, m.width)
}

// workInFlight reports whether a browse surface or the detail is loading, on
// screen or not, or the store picker while it is up: it is what draws the
// header spinner and arms its tick.
func (m Model) workInFlight() bool {
	for _, entry := range m.browseTabs() {
		if entry.Tab.IsLoading() {
			return true
		}
	}
	if m.detail.IsLoading() {
		return true
	}
	// The picker draws its own spinner, and this is what arms the tick that
	// advances it. Reported only while the picker is on screen: a listing still
	// in flight after the operator switched to a browse tab would otherwise spin
	// that tab's header for a surface nobody is looking at.
	return m.active == mode.StorePicker && m.storePicker != nil && m.storePicker.IsLoading()
}

func shellKeyHelp(keys config.ResolvedKeyBindings) string {
	return strings.Join([]string{
		"Mode switching:",
		fmt.Sprintf("  %s/%s = next/previous tab (Board, Docs)", keys.DisplayLabel(config.ShellContext, config.ShellActionModeCycleNext), keys.DisplayLabel(config.ShellContext, config.ShellActionModeCyclePrev)),
		"",
		"Selection:",
		fmt.Sprintf("  Board: %s switch columns, %s move within a column", combineDisplayLabels(keys, config.BoardContext, config.BoardActionMoveLeft, config.BoardActionMoveRight), combineDisplayLabels(keys, config.BoardContext, config.BoardActionMoveUp, config.BoardActionMoveDown)),
		fmt.Sprintf("  Docs: %s move within the doc list (docs use the board keymap)", combineDisplayLabels(keys, config.BoardContext, config.BoardActionMoveUp, config.BoardActionMoveDown)),
		"  Board and Docs: type to filter the rows by title or ID; every word must match",
		fmt.Sprintf("  backspace, ctrl+w and ctrl+u edit the filter, %s clears it", keys.DisplayLabel(config.ShellContext, config.ShellActionEscape)),
		fmt.Sprintf("  Search: %s opens the store search; type to search titles, IDs and descriptions", keys.DisplayLabel(config.ShellContext, config.ShellActionOpenSearch)),
		fmt.Sprintf("  ctrl+t switches it between open issues and all of them, %s clears the query and then goes back", keys.DisplayLabel(config.ShellContext, config.ShellActionEscape)),
		fmt.Sprintf("  Board, Docs, Search and the store list: %s/%s move a page, %s/%s go to the first and last row", keys.DisplayLabel(config.BoardContext, config.BoardActionPageUp), keys.DisplayLabel(config.BoardContext, config.BoardActionPageDown), keys.DisplayLabel(config.BoardContext, config.BoardActionMoveHome), keys.DisplayLabel(config.BoardContext, config.BoardActionMoveEnd)),
		"",
		"Actions:",
		fmt.Sprintf("  %s = create issue (inline modal)", keys.DisplayLabel(config.ShellContext, config.ShellActionCreateIssue)),
		fmt.Sprintf("  %s = update selected issue metadata", keys.DisplayLabel(config.ShellContext, config.ShellActionUpdateIssue)),
		fmt.Sprintf("  %s = close selected issue", keys.DisplayLabel(config.ShellContext, config.ShellActionCloseIssue)),
		fmt.Sprintf("  %s = add comment to selected issue", keys.DisplayLabel(config.ShellContext, config.ShellActionCommentIssue)),
		fmt.Sprintf("  %s = edit selected issue in external editor", keys.DisplayLabel(config.ShellContext, config.ShellActionEditIssue)),
		fmt.Sprintf("  %s/%s/%s = launch external tools (detail mode, background fire-and-forget)", keys.DisplayLabel(config.ShellContext, config.ShellActionLaunchNvim), keys.DisplayLabel(config.ShellContext, config.ShellActionLaunchOpencode), keys.DisplayLabel(config.ShellContext, config.ShellActionLaunchShell)),
		"  launcher actions do not provide in-app return/save handling",
		fmt.Sprintf("  use %s for edit/save round-trip that reloads detail", keys.DisplayLabel(config.ShellContext, config.ShellActionEditIssue)),
		fmt.Sprintf("  %s = open selected issue in detail mode", keys.DisplayLabel(config.BoardContext, config.BoardActionOpenDetail)),
		fmt.Sprintf("  scroll detail and this help: %s/%s, %s/%s, %s/%s", keys.DisplayLabel(config.DetailContext, config.DetailActionScrollDown), keys.DisplayLabel(config.DetailContext, config.DetailActionScrollUp), keys.DisplayLabel(config.DetailContext, config.DetailActionPageUp), keys.DisplayLabel(config.DetailContext, config.DetailActionPageDown), keys.DisplayLabel(config.DetailContext, config.DetailActionHome), keys.DisplayLabel(config.DetailContext, config.DetailActionEnd)),
		fmt.Sprintf("  %s = reload detail mode from repository", keys.DisplayLabel(config.ShellContext, config.ShellActionReloadDetail)),
		fmt.Sprintf("  %s = return from detail to browse / dismiss toast", keys.DisplayLabel(config.ShellContext, config.ShellActionEscape)),
		fmt.Sprintf("  %s = list the central task stores on this machine and open one", keys.DisplayLabel(config.ShellContext, config.ShellActionStorePicker)),
		fmt.Sprintf("  %s = toggle help", keys.DisplayLabel(config.ShellContext, config.ShellActionHelp)),
		fmt.Sprintf("  %s = quit", keys.DisplayLabel(config.ShellContext, config.ShellActionQuit)),
		"",
		"Mouse:",
		"  click = select a row, switch to a tab, press a menu-bar button, or focus a pane",
		"  second click on a row = open it",
		"  wheel = move the selection, or scroll detail text and this help",
		"  drag = select a box of text; letting go sends it to the terminal clipboard",
		"  shift+drag = select text with the terminal instead",
		"",
		"Detail presentation model (v1): dedicated detail mode",
		"  - Board prioritizes overview triage density",
		fmt.Sprintf("  - %s opens full issue detail view", keys.DisplayLabel(config.BoardContext, config.BoardActionOpenDetail)),
	}, "\n")
}

// storePickerHints is the picker's own legend. The shell footer is not
// rendered while the picker is up, so this line is the only place its keys are
// named on screen. With no store open, Escape quits rather than going back.
func storePickerHints(keys config.ResolvedKeyBindings, storeOpen bool) []styles.KeyHint {
	escape := "back"
	if !storeOpen {
		escape = "quit"
	}
	primary := keys.DisplayPrimary
	return []styles.KeyHint{
		{Key: primary(config.BoardContext, config.BoardActionMoveDown) + "/" + primary(config.BoardContext, config.BoardActionMoveUp), Desc: "move"},
		{Key: primary(config.BoardContext, config.BoardActionOpenDetail), Desc: "open"},
		{Key: primary(config.BoardContext, config.BoardActionReload), Desc: "reload"},
		{Key: primary(config.ShellContext, config.ShellActionEscape), Desc: escape},
		{Key: primary(config.ShellContext, config.ShellActionQuit), Desc: "quit"},
	}
}

func combineDisplayLabels(keys config.ResolvedKeyBindings, context, first, second string) string {
	left := keys.DisplayLabel(context, first)
	right := keys.DisplayLabel(context, second)
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	return left + " or " + right
}

// footerHints is the legend of the active surface, the keys an operator needs
// first leading: KeyLegend drops from the end.
func footerHints(active mode.ID, keys config.ResolvedKeyBindings) []styles.KeyHint {
	primary := keys.DisplayPrimary
	pair := func(context, first, second string) string {
		return primary(context, first) + "/" + primary(context, second)
	}
	help := styles.KeyHint{Key: primary(config.ShellContext, config.ShellActionHelp), Desc: "help"}
	quit := styles.KeyHint{Key: primary(config.ShellContext, config.ShellActionQuit), Desc: "quit"}
	// Creating an issue has no button: the legend is where its key is named.
	create := styles.KeyHint{Key: primary(config.ShellContext, config.ShellActionCreateIssue), Desc: "new"}
	// No key enters the filter, so the legend names the typing itself.
	filter := styles.KeyHint{Key: "type", Desc: "filter"}
	clear := styles.KeyHint{Key: primary(config.ShellContext, config.ShellActionEscape), Desc: "clear"}

	switch active {
	case mode.Docs:
		return []styles.KeyHint{
			{Key: pair(config.BoardContext, config.BoardActionMoveDown, config.BoardActionMoveUp), Desc: "docs"},
			{Key: primary(config.BoardContext, config.BoardActionOpenDetail), Desc: "detail"},
			filter, clear,
			create,
			{Key: pair(config.ShellContext, config.ShellActionModeCycleNext, config.ShellActionModeCyclePrev), Desc: "tabs"},
			help, quit,
		}
	case mode.Search:
		return []styles.KeyHint{
			{Key: pair(config.BoardContext, config.BoardActionMoveDown, config.BoardActionMoveUp), Desc: "results"},
			{Key: primary(config.BoardContext, config.BoardActionOpenDetail), Desc: "detail"},
			{Key: "type", Desc: "search"},
			{Key: "ctrl+t", Desc: "open/all"},
			{Key: primary(config.ShellContext, config.ShellActionEscape), Desc: "clear, back"},
			create,
			{Key: pair(config.ShellContext, config.ShellActionModeCycleNext, config.ShellActionModeCyclePrev), Desc: "tabs"},
			help, quit,
		}
	case mode.Detail:
		return []styles.KeyHint{
			{Key: pair(config.DetailContext, config.DetailActionScrollDown, config.DetailActionScrollUp), Desc: "scroll"},
			{Key: primary(config.ShellContext, config.ShellActionEscape), Desc: "back"},
			{Key: primary(config.ShellContext, config.ShellActionEditIssue), Desc: "edit"},
			{Key: pair(config.DetailContext, config.DetailActionPageUp, config.DetailActionPageDown), Desc: "page"},
			{Key: pair(config.DetailContext, config.DetailActionHome, config.DetailActionEnd), Desc: "bounds"},
		}
	default:
		return []styles.KeyHint{
			{Key: pair(config.BoardContext, config.BoardActionMoveLeft, config.BoardActionMoveRight), Desc: "columns"},
			{Key: pair(config.BoardContext, config.BoardActionMoveDown, config.BoardActionMoveUp), Desc: "issues"},
			{Key: primary(config.BoardContext, config.BoardActionOpenDetail), Desc: "detail"},
			filter, clear,
			create,
			help, quit,
		}
	}
}
