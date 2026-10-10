package app

import (
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

// The screen margin: the blank cells between the terminal's edge and
// everything the app draws. A terminal under marginMinWidth columns has none,
// and one under marginMinHeight rows keeps the side columns and gives up the
// rows.
const (
	screenMarginRows = 1
	screenMarginCols = 2
	marginMinWidth   = 100
	marginMinHeight  = 24
)

// setTerminalSize takes the terminal's size apart into the margin and the
// screen inside it. Nothing else reads the terminal's size.
func (m *Model) setTerminalSize(msg tea.WindowSizeMsg) {
	m.sizeKnown = true
	m.marginRows, m.marginCols = 0, 0
	if msg.Width >= marginMinWidth {
		m.marginCols = screenMarginCols
		if msg.Height >= marginMinHeight {
			m.marginRows = screenMarginRows
		}
	}
	m.width = msg.Width - 2*m.marginCols
	m.height = msg.Height - 2*m.marginRows
}

// View renders the screen inside the margin. It is the only place the margin
// is drawn: every surface, overlay and toast is on the screen, and so inside
// it.
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

	lines := strings.Split(m.screen(), "\n")
	left := strings.Repeat(" ", m.marginCols)
	for idx := range lines {
		lines[idx] = left + lines[idx]
	}
	blank := strings.Repeat("\n", m.marginRows)
	return blank + strings.Join(lines, "\n") + blank
}

// screen renders the root shell at the size the margin leaves.
func (m Model) screen() string {
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

	return view
}

// renderSurface renders whatever surface is active, without overlays.
//
// The picker is not a tab and not a drill-in: it renders instead of the shell,
// so the header and the legend are absent while it is up and it draws its own
// legend (docs/DESIGN-GUIDE.md). The configuration screen is the same, and so
// is the help screen, which stands in the place of whatever surface is active.
func (m Model) renderSurface() string {
	if m.showHelp {
		return firstLines(m.renderHelp(), m.height)
	}

	switch m.active {
	case mode.StorePicker:
		return firstLines(m.storePicker.View(m.spinnerFrame, styles.KeyLegend(storePickerHints(m.keys, m.storeOpen), m.width)), m.height)
	case mode.Config:
		return firstLines(m.configScreen.View(version.Version, styles.KeyLegend(configScreenHints(m.keys), styles.ScreenTextWidth(m.width))), m.height)
	}

	// A renderer keeps a floor of rows however short the workspace is. The body
	// is cut to the workspace, so the header stays on the screen's row 0 and
	// the legend on the row handleMouse takes for it: Bubble Tea drops the top
	// of a frame taller than the terminal, and every click then lands a row
	// off.
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

	headerSpinnerCols = 2
	headerTabPadding  = 1
	headerTabGap      = 1
	// headerBarGap is the least space between the last button and the version.
	headerBarGap = 2
	// storeLabelMax is the widest a store name is drawn on the menu bar, and
	// storeLabelFloor the narrowest it is cut to before a button is dropped.
	storeLabelMax   = 24
	storeLabelFloor = 8
	// barSeparator stands between two buttons.
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
	{label: barLabel("config"), key: shellKey(config.ShellActionOpenConfig), run: (*Model).openConfig},
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

// barCell is a button's place on the bar: its text, and the columns it covers,
// the space on each side of it included.
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

// barCells places the buttons from the left edge: the first button's own
// leading space is the line's gutter. A bar that does not fit
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
	used := (len(actions) - 1) * lipgloss.Width(barSeparator)
	for _, action := range actions {
		cell := barCell{action: action, label: action.label(m), key: action.key(m)}
		cells = append(cells, cell)
		used += lipgloss.Width(cell.text())
	}

	x := 0
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

// runs is a button's two runs as they are drawn: the label, and the key after
// it where the action has one. A space stands on each side of the button, so
// the pointer lights, and a click takes, a cell wider than the words.
func (c barCell) runs() (label, key string) {
	if c.key == "" {
		return " " + c.label + " ", ""
	}
	return " " + c.label + " ", c.key + " "
}

func (c barCell) text() string {
	label, key := c.runs()
	return label + key
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
// The version is the first to go when the two do not fit. The button under the
// pointer takes the hover background over its whole cell, a step below the
// selected row's, and a bold label.
func (m Model) renderMenuBar() string {
	label := lipgloss.NewStyle().Foreground(styles.ShellActionColor)
	muted := lipgloss.NewStyle().Foreground(styles.ShellFooterHelpColor)

	cells := m.barCells()
	bar := ""
	for idx, cell := range cells {
		if idx > 0 {
			bar += muted.Render(barSeparator)
		}
		labelStyle, keyStyle := label.Bold(cell.action.name), muted
		if m.barPointer != nil && cell.covers(*m.barPointer) {
			labelStyle = label.Background(styles.ShellHoverBgColor).Bold(true)
			keyStyle = muted.Background(styles.ShellHoverBgColor)
		}
		text, key := cell.runs()
		bar += labelStyle.Render(text)
		if key != "" {
			bar += keyStyle.Render(key)
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
	return styles.InsetRule(m.width)
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
			base = base.Foreground(styles.ShellTabHoverColor).Background(styles.ShellHoverBgColor)
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

// workspaceHeight is the rows the screen has left between a rendered header
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

// renderFooter draws the key legend, one cell in. It is one line whatever the
// width: the workspace height is computed from it, so a hint that does not fit
// is dropped rather than wrapped.
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
	// The legend starts under the label of the first button, as it does on a
	// full screen, so it stays in place when one opens.
	return textutil.TruncateString(" "+styles.KeyLegend(hints, max(1, styles.ScreenTextWidth(m.width))), m.width)
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

// configScreenHints is the configuration screen's own legend, for the reason
// the picker has one. Left, Right and Enter are built in there, so the legend
// names the keys themselves rather than a binding.
func configScreenHints(keys config.ResolvedKeyBindings) []styles.KeyHint {
	primary := keys.DisplayPrimary
	return []styles.KeyHint{
		{Key: primary(config.BoardContext, config.BoardActionMoveDown) + "/" + primary(config.BoardContext, config.BoardActionMoveUp), Desc: "move"},
		{Key: "left/right", Desc: "change"},
		{Key: primary(config.ShellContext, config.ShellActionEscape), Desc: "back"},
		{Key: primary(config.ShellContext, config.ShellActionQuit), Desc: "quit"},
	}
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
