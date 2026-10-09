package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/ui/fatalerror"
	"github.com/hk9890/task-manager-ui/internal/ui/loading"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
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
// so the top bar and the legend are absent while it is up and it draws its own
// legend (docs/DESIGN-GUIDE.md).
func (m Model) renderSurface() string {
	if m.active == mode.StorePicker {
		return m.storePicker.View(m.spinnerFrame, styles.KeyLegend(storePickerHints(m.keys, m.storeOpen), m.width))
	}

	return lipgloss.JoinVertical(lipgloss.Left, m.renderHeader(), m.renderBody(), m.renderFooter())
}

// The top bar's geometry. renderHeader draws from it, and tabAt and barCells
// read a column back through it, so a click lands on what is drawn there.
const (
	headerSpinnerCols = 2
	headerTabPadding  = 1
	headerTabGap      = 1
	// headerBarGap is the least space between the last tab and the first
	// button; a button that would close it is dropped.
	headerBarGap = 2
	// barSeparator stands between two buttons, as it does between two key
	// hints on the legend.
	barSeparator = " · "
	ruleGlyph    = "─"
)

// tabLabels names each browse tab on the top bar.
var tabLabels = map[mode.ID]string{
	mode.Board:  "Board",
	mode.Docs:   "Docs",
	mode.Search: "Search",
}

// barAction is one button on the right of the top bar: what the shell can do
// that is not about the row under the cursor. A button carries the key bound
// to its action — the bar is a second way to reach the same thing, never the
// only way.
type barAction struct {
	label  string
	action string
	run    func(*Model) tea.Cmd
}

var barActions = []barAction{
	{label: "new", action: config.ShellActionCreateIssue, run: (*Model).requestCreateIssue},
	{label: "stores", action: config.ShellActionStorePicker, run: (*Model).openStorePicker},
	{label: "help", action: config.ShellActionHelp, run: (*Model).openHelp},
}

// barCell is a button's place on the bar: its text, and the columns it covers.
type barCell struct {
	action barAction
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

// barCells places the buttons flush right. The tabs come first: a button that
// does not fit beside them is dropped, the leftmost first, so help is the last
// to go.
func (m Model) barCells() []barCell {
	cells := make([]barCell, 0, len(barActions))
	for _, action := range barActions {
		cells = append(cells, barCell{action: action, key: m.keys.DisplayPrimary(config.ShellContext, action.action)})
	}

	free := m.width - headerTabsEnd() - headerBarGap
	for len(cells) > 0 {
		total := (len(cells) - 1) * lipgloss.Width(barSeparator)
		for _, cell := range cells {
			total += lipgloss.Width(cell.text())
		}
		if total > free {
			cells = cells[1:]
			continue
		}
		x := m.width - total
		for idx := range cells {
			cells[idx].x0 = x
			cells[idx].x1 = x + lipgloss.Width(cells[idx].text())
			x = cells[idx].x1 + lipgloss.Width(barSeparator)
		}
		break
	}
	return cells
}

func (c barCell) text() string {
	if c.key == "" {
		return c.action.label
	}
	return c.action.label + " " + c.key
}

// headerSpinnerCell returns a fixed 2-cell string: the current spinner glyph
// followed by a space when any surface is loading, or two literal spaces when
// idle. Using a fixed-width cell keeps the tabs where tabAt looks for them.
func (m Model) headerSpinnerCell() string {
	style := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	if len(m.loadingStates()) > 0 {
		return style.Render(loading.Glyph(m.spinnerFrame) + " ")
	}
	return style.Render("  ")
}

// renderHeader draws the two lines above the workspace: the top bar — the tabs
// on the left, the buttons on the right — and the rule that carries the
// context.
func (m Model) renderHeader() string {
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
	bar := lipgloss.JoinHorizontal(lipgloss.Top, parts...)

	if cells := m.barCells(); len(cells) > 0 {
		label := lipgloss.NewStyle().Foreground(styles.ShellActionColor)
		key := lipgloss.NewStyle().Foreground(styles.ShellFooterHelpColor)
		buttons := make([]string, 0, len(cells))
		for _, cell := range cells {
			labelStyle := label
			if m.hoverAction == cell.action.action {
				labelStyle = label.Foreground(styles.ShellTabHoverColor).Bold(true)
			}
			button := labelStyle.Render(cell.action.label)
			if cell.key != "" {
				button += " " + key.Render(cell.key)
			}
			buttons = append(buttons, button)
		}
		bar += strings.Repeat(" ", cells[0].x0-lipgloss.Width(bar)) + strings.Join(buttons, key.Render(barSeparator))
	}

	return bar + "\n" + m.renderRule()
}

// renderRule is the line under the top bar: a rule, with the store and what
// the surface shows set into its left end.
func (m Model) renderRule() string {
	context := m.headerContext()
	contextStyle := lipgloss.NewStyle().Foreground(styles.ShellContextColor)
	if m.width <= 0 {
		return contextStyle.Render(context)
	}

	rule := lipgloss.NewStyle().Foreground(styles.ShellRuleColor)
	lead := strings.Repeat(ruleGlyph, 2) + " "
	context = textutil.TruncateString(context, m.width-lipgloss.Width(lead)-1)
	if context == "" {
		return rule.Render(strings.Repeat(ruleGlyph, m.width))
	}
	fill := max(0, m.width-lipgloss.Width(lead)-lipgloss.Width(context)-1)
	return rule.Render(lead) + contextStyle.Render(context) + " " + rule.Render(strings.Repeat(ruleGlyph, fill))
}

// renderBody renders the active surface. Like every other renderer here it is
// pure: sizing runs from the WindowSizeMsg handler and the search preview's
// detail is synced from Update, because selection/detail sync is event-driven
// and not polled (docs/CODING.md, Core Architectural Rule 9). Mutating model
// state from View() worked only for as long as something re-rendered often
// enough, which is exactly what the always-on spinner tick was doing.
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

func (m *Model) syncSearchPreviewDetailState() {
	if m.search == nil {
		return
	}
	// ResultCount, not SessionState().Page: this runs on every Update, and
	// SessionState deep-copies the whole result page to answer "is it empty".
	if m.search.ResultCount() == 0 {
		m.search.SetSelectedDetail(domain.IssueDetail{}, false)
		return
	}
	selection := m.selectedByMode[mode.Search]
	if selection == nil || strings.TrimSpace(selection.Issue.ID) == "" {
		m.search.SetSelectedDetail(domain.IssueDetail{}, false)
		return
	}

	selectedID := strings.TrimSpace(selection.Issue.ID)
	if m.detail.IsLoading() && strings.TrimSpace(m.detail.TargetID()) == selectedID {
		m.search.SetSelectedDetail(domain.IssueDetail{}, true)
		return
	}
	if strings.TrimSpace(m.detail.Detail.Summary.ID) == selectedID && !m.detail.IsLoading() && strings.TrimSpace(m.detail.Error()) == "" {
		m.search.SetSelectedDetail(m.detail.Detail, false)
		return
	}

	m.search.SetSelectedDetail(domain.IssueDetail{}, false)
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
	workspaceWidth := max(1, m.width)
	headerHeight := lipgloss.Height(m.renderHeader())
	footerHeight := lipgloss.Height(m.renderFooter())
	workspaceHeight := max(1, m.height-headerHeight-footerHeight)
	return workspaceWidth, workspaceHeight
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

// browseLoadingScope maps a browse mode to its loading scope. A new browse
// surface needs its own scope, or its work reports as somebody else's
// (DESIGN-GUIDE.md).
func browseLoadingScope(id mode.ID) loading.Scope {
	switch id {
	case mode.Board:
		return loading.ScopeBoard
	case mode.Docs:
		return loading.ScopeDocs
	case mode.Search:
		return loading.ScopeSearch
	}
	return loading.Scope(id)
}

func (m Model) loadingStates() []loading.State {
	loadingStates := make([]loading.State, 0, 4)
	for _, entry := range m.browseTabs() {
		if entry.Tab.IsLoading() {
			loadingStates = append(loadingStates, loading.State{Scope: browseLoadingScope(entry.ID)})
		}
	}
	if m.detail.IsLoading() {
		loadingStates = append(loadingStates, loading.State{Scope: loading.ScopeDetail, Target: m.detail.TargetID()})
	}
	// The picker draws its own spinner, and this is what arms the tick that
	// advances it. Reported only while the picker is on screen: a listing still
	// in flight after the operator switched to a browse tab would otherwise spin
	// that tab's header for a surface nobody is looking at.
	if m.active == mode.StorePicker && m.storePicker != nil && m.storePicker.IsLoading() {
		loadingStates = append(loadingStates, loading.State{Scope: loading.ScopeStores})
	}
	return loadingStates
}

func (m Model) headerContext() string {
	variants := m.headerContextVariants()
	if len(variants) == 0 {
		return ""
	}

	if m.width <= 0 {
		return variants[0]
	}

	for _, v := range variants {
		if lipgloss.Width(v) <= m.width/2 {
			return v
		}
	}

	return variants[len(variants)-1]
}

// headerContextVariants returns the right-hand header text, widest first.
//
// With stores switchable, which store is on screen is the first thing the
// header must say, so every variant leads with its name and the name survives
// narrowing until nothing but the surface fits beside it. The unnamed variants
// stay as the last resort for a terminal too narrow even for that.
func (m Model) headerContextVariants() []string {
	variants := m.surfaceContextVariants()
	name := strings.TrimSpace(m.services.StoreName)
	if name == "" {
		return variants
	}

	named := make([]string, 0, 2*len(variants))
	for _, variant := range variants {
		named = append(named, name+" · "+variant)
	}
	return append(named, variants...)
}

func (m Model) surfaceContextVariants() []string {
	if m.active == mode.Detail {
		id := strings.TrimSpace(m.detail.Detail.Summary.ID)
		if id == "" {
			id = strings.TrimSpace(m.detail.SelectionID())
		}
		status := strings.TrimSpace(m.detail.Detail.Summary.Status)
		if id != "" && status != "" {
			return []string{fmt.Sprintf("Detail: %s · %s", id, status), fmt.Sprintf("Detail: %s", id), "Detail"}
		}
		if id != "" {
			return []string{fmt.Sprintf("Detail: %s", id), "Detail"}
		}
		return []string{"Detail"}
	}

	prefix := "Board"
	switch m.active {
	case mode.Docs:
		prefix = "Docs"
	case mode.Search:
		// Ask the mode directly: SessionState() deep-copies the whole result
		// page, which is not worth doing on the render path for one integer.
		count := 0
		if m.search != nil {
			count = m.search.ResultCount()
		}
		prefix = fmt.Sprintf("Search: %d results", count)
	}

	selectedLong, selectedShort := "Selected: none", "Sel: none"
	if sel := m.currentSelection(); sel != nil {
		selectedLong = fmt.Sprintf("Selected: %s (%s)", sel.Issue.ID, sel.Issue.Status)
		selectedShort = fmt.Sprintf("Sel: %s", sel.Issue.ID)
	}

	loadingSummary := loading.Summary(m.loadingStates())
	loadingShort := loadingSummary
	if loadingSummary == "Idle" {
		loadingShort = "idle"
	}

	variants := []string{
		fmt.Sprintf("%s · %s · %s", prefix, selectedLong, loadingSummary),
		fmt.Sprintf("%s · %s", prefix, selectedLong),
		fmt.Sprintf("%s · %s · %s", prefix, selectedShort, loadingShort),
		prefix,
	}

	if m.active == mode.Search {
		variants = append(variants, []string{
			fmt.Sprintf("Search · %s · %s", selectedLong, loadingSummary),
			fmt.Sprintf("Search · %s", selectedShort),
		}...)
	}

	return variants
}

func shellKeyHelp(keys config.ResolvedKeyBindings) string {
	return strings.Join([]string{
		"Mode switching:",
		fmt.Sprintf("  %s/%s = next/previous tab (Board, Docs, Search)", keys.DisplayLabel(config.ShellContext, config.ShellActionModeCycleNext), keys.DisplayLabel(config.ShellContext, config.ShellActionModeCyclePrev)),
		fmt.Sprintf("  %s = toggle Board/Search", keys.DisplayLabel(config.ShellContext, config.ShellActionToggleSearch)),
		fmt.Sprintf("  %s = open selected issue detail", keys.DisplayLabel(config.ShellContext, config.ShellActionModeDetail)),
		"",
		"Selection:",
		fmt.Sprintf("  Board: %s switch columns, %s move within a column", combineDisplayLabels(keys, config.BoardContext, config.BoardActionMoveLeft, config.BoardActionMoveRight), combineDisplayLabels(keys, config.BoardContext, config.BoardActionMoveUp, config.BoardActionMoveDown)),
		fmt.Sprintf("  Docs: %s move within the doc list (docs use the board keymap)", combineDisplayLabels(keys, config.BoardContext, config.BoardActionMoveUp, config.BoardActionMoveDown)),
		fmt.Sprintf("  Search: type query text, then Enter to search; %s focuses query; %s/%s switch panes; %s/%s moves query/results and result selection; %s/%s cycles focus; ctrl+t widens the scope to closed issues and back", keys.DisplayLabel(config.SearchContext, config.SearchActionFocusQuery), keys.DisplayLabel(config.SearchContext, config.SearchActionFocusLeft), keys.DisplayLabel(config.SearchContext, config.SearchActionFocusRight), keys.DisplayLabel(config.SearchContext, config.SearchActionMoveDown), keys.DisplayLabel(config.SearchContext, config.SearchActionMoveUp), keys.DisplayLabel(config.SearchContext, config.SearchActionCycleFocusNext), keys.DisplayLabel(config.SearchContext, config.SearchActionCycleFocusPrev)),
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
		fmt.Sprintf("  detail scroll: %s/%s, %s/%s, %s/%s", keys.DisplayLabel(config.DetailContext, config.DetailActionScrollDown), keys.DisplayLabel(config.DetailContext, config.DetailActionScrollUp), keys.DisplayLabel(config.DetailContext, config.DetailActionPageUp), keys.DisplayLabel(config.DetailContext, config.DetailActionPageDown), keys.DisplayLabel(config.DetailContext, config.DetailActionHome), keys.DisplayLabel(config.DetailContext, config.DetailActionEnd)),
		fmt.Sprintf("  %s = reload detail mode from repository", keys.DisplayLabel(config.ShellContext, config.ShellActionReloadDetail)),
		fmt.Sprintf("  %s = return from detail/search to browse / dismiss toast", keys.DisplayLabel(config.ShellContext, config.ShellActionEscape)),
		fmt.Sprintf("  %s = list the central task stores on this machine and open one", keys.DisplayLabel(config.ShellContext, config.ShellActionStorePicker)),
		fmt.Sprintf("  %s = toggle help", keys.DisplayLabel(config.ShellContext, config.ShellActionHelp)),
		fmt.Sprintf("  %s = quit", keys.DisplayLabel(config.ShellContext, config.ShellActionQuit)),
		"",
		"Mouse:",
		"  click = select a row, switch to a tab, or focus a pane",
		"  second click on a row = open it",
		"  wheel = move the selection, or scroll detail text and this help",
		"  drag = select a box of text; letting go copies it",
		"  shift+drag = select text with the terminal instead",
		"",
		"Detail presentation model (v1): dedicated detail mode",
		"  - Board/Search prioritize overview triage density",
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

	switch active {
	case mode.Docs:
		return []styles.KeyHint{
			{Key: pair(config.BoardContext, config.BoardActionMoveDown, config.BoardActionMoveUp), Desc: "docs"},
			{Key: primary(config.BoardContext, config.BoardActionOpenDetail), Desc: "detail"},
			{Key: pair(config.ShellContext, config.ShellActionModeCycleNext, config.ShellActionModeCyclePrev), Desc: "tabs"},
			help, quit,
		}
	case mode.Search:
		return []styles.KeyHint{
			{Key: "type + enter", Desc: "query"},
			{Key: primary(config.SearchContext, config.SearchActionFocusQuery), Desc: "focus"},
			{Key: pair(config.SearchContext, config.SearchActionMoveDown, config.SearchActionMoveUp), Desc: "results"},
			{Key: primary(config.SearchContext, config.SearchActionOpenDetail), Desc: "detail"},
			{Key: primary(config.ShellContext, config.ShellActionEscape), Desc: "board"},
			{Key: pair(config.SearchContext, config.SearchActionFocusLeft, config.SearchActionFocusRight), Desc: "panes"},
			{Key: "ctrl+t", Desc: "scope"},
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
			{Key: primary(config.ShellContext, config.ShellActionToggleSearch), Desc: "search"},
			help, quit,
		}
	}
}
