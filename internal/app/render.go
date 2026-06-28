package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/ui/fatalerror"
	"github.com/hk9890/task-manager-ui/internal/ui/loading"
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

	if m.fatalErrTitle != "" {
		return fatalerror.Render(fatalerror.State{
			Title:  m.fatalErrTitle,
			Body:   m.fatalErrBody,
			Width:  m.width,
			Height: m.height,
		})
	}

	header := m.renderHeader()
	body := m.renderBody()
	footer := m.renderFooter()

	view := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
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

// headerSpinnerCell returns a fixed 2-cell string: the current braille spinner
// glyph followed by a space when any surface is loading, or two literal spaces
// when idle. Using a fixed-width cell keeps lipgloss.Width(headerLeft) invariant.
func (m Model) headerSpinnerCell() string {
	style := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	if len(m.loadingStates()) > 0 {
		return style.Render(loading.Glyph(m.spinnerFrame) + " ")
	}
	return style.Render("  ")
}

func (m Model) renderHeader() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(styles.ShellTitleColor).Render("Task Manager UI")

	tab := func(id mode.ID, label string) string {
		base := lipgloss.NewStyle().Padding(0, 1)
		if m.active == id {
			return base.Foreground(styles.ShellTabActiveTextColor).Background(styles.ShellTabActiveBgColor).Bold(true).Render(label)
		}
		return base.Foreground(styles.ShellTabInactiveColor).Render(label)
	}

	left := lipgloss.JoinHorizontal(
		lipgloss.Top,
		title,
		m.headerSpinnerCell(),
		tab(mode.Board, "Board"),
		" ",
		tab(mode.Search, "Search"),
	)

	context := lipgloss.NewStyle().Foreground(styles.ShellContextColor).Render(m.headerContext())
	if m.width <= 0 {
		return left
	}

	leftWidth := lipgloss.Width(left)
	contextWidth := lipgloss.Width(context)
	if leftWidth+1+contextWidth > m.width {
		available := max(0, m.width-leftWidth-1)
		if available <= 0 {
			return left
		}
		context = lipgloss.NewStyle().Foreground(styles.ShellContextColor).Render(styles.TruncateString(m.headerContext(), available))
		contextWidth = lipgloss.Width(context)
	}

	spacer := strings.Repeat(" ", max(1, m.width-leftWidth-contextWidth))
	return left + spacer + context
}

func (m Model) renderBody() string {
	workspaceWidth, workspaceHeight := m.workspaceSize()
	m.board.SetSize(workspaceWidth, workspaceHeight)
	m.search.SetSize(workspaceWidth, workspaceHeight)
	m.syncSearchPreviewDetailState()

	skeletonPhase := loading.SkeletonPhase(m.spinnerFrame)

	if m.active == mode.Detail {
		return m.detail.View(m.detailViewportWidth(), m.detailViewportHeight(), false, skeletonPhase)
	}

	var browse string
	if m.active == mode.Board {
		browse = m.board.View(skeletonPhase)
	} else {
		browse = m.search.View(skeletonPhase)
	}

	return browse
}

func (m *Model) syncSearchPreviewDetailState() {
	if m.search == nil {
		return
	}
	session := m.search.SessionState()
	if len(session.Page.Results) == 0 {
		m.search.SetSelectedDetail(domain.IssueDetail{}, false)
		return
	}
	selection := m.selectedByMode[mode.Search]
	if selection == nil || strings.TrimSpace(selection.Issue.ID) == "" {
		m.search.SetSelectedDetail(domain.IssueDetail{}, false)
		return
	}

	selectedID := strings.TrimSpace(selection.Issue.ID)
	if m.detail.Loading && strings.TrimSpace(m.detail.TargetID) == selectedID {
		m.search.SetSelectedDetail(domain.IssueDetail{}, true)
		return
	}
	if strings.TrimSpace(m.detail.Detail.Summary.ID) == selectedID && !m.detail.Loading && strings.TrimSpace(m.detail.Error) == "" {
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
	m.board.SetSize(workspaceWidth, workspaceHeight)
	m.search.SetSize(workspaceWidth, workspaceHeight)
}

func (m Model) renderFooter() string {
	if !m.services.Config.UI.ShowModeSwitcherHelp {
		return ""
	}

	return lipgloss.NewStyle().Foreground(styles.ShellFooterHelpColor).Render(footerHelpText(m.active, m.width, m.keys))
}

func (m Model) loadingStates() []loading.State {
	loadingStates := make([]loading.State, 0, 3)
	if m.boardIsLoading() {
		loadingStates = append(loadingStates, loading.State{Scope: loading.ScopeBoard})
	}
	if m.searchIsLoading() {
		loadingStates = append(loadingStates, loading.State{Scope: loading.ScopeSearch})
	}
	if m.detail.Loading {
		loadingStates = append(loadingStates, loading.State{Scope: loading.ScopeDetail, Target: m.detail.TargetID})
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

func (m Model) headerContextVariants() []string {
	if m.active == mode.Detail {
		id := strings.TrimSpace(m.detail.Detail.Summary.ID)
		if id == "" {
			id = strings.TrimSpace(m.detail.SelectionID)
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
	if m.active == mode.Search {
		prefix = fmt.Sprintf("Search: %d results", m.searchResultCount())
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
		fmt.Sprintf("  %s = toggle Board/Search", keys.DisplayLabel(config.ShellContext, config.ShellActionToggleSearch)),
		fmt.Sprintf("  %s = open selected issue detail", keys.DisplayLabel(config.ShellContext, config.ShellActionModeDetail)),
		"",
		"Selection:",
		fmt.Sprintf("  Board: %s switch columns, %s move within a column", combineDisplayLabels(keys, config.BoardContext, config.BoardActionMoveLeft, config.BoardActionMoveRight), combineDisplayLabels(keys, config.BoardContext, config.BoardActionMoveUp, config.BoardActionMoveDown)),
		fmt.Sprintf("  Search: type query text, then Enter to search; %s focuses query; %s/%s switch panes; %s/%s moves query/results and result selection; %s/%s cycles focus", keys.DisplayLabel(config.SearchContext, config.SearchActionFocusQuery), keys.DisplayLabel(config.SearchContext, config.SearchActionFocusLeft), keys.DisplayLabel(config.SearchContext, config.SearchActionFocusRight), keys.DisplayLabel(config.SearchContext, config.SearchActionMoveDown), keys.DisplayLabel(config.SearchContext, config.SearchActionMoveUp), keys.DisplayLabel(config.SearchContext, config.SearchActionCycleFocusNext), keys.DisplayLabel(config.SearchContext, config.SearchActionCycleFocusPrev)),
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
		fmt.Sprintf("  %s = toggle help", keys.DisplayLabel(config.ShellContext, config.ShellActionHelp)),
		fmt.Sprintf("  %s = quit", keys.DisplayLabel(config.ShellContext, config.ShellActionQuit)),
		"",
		"Detail presentation model (v1): dedicated detail mode",
		"  - Board/Search prioritize overview triage density",
		fmt.Sprintf("  - %s opens full issue detail view", keys.DisplayLabel(config.BoardContext, config.BoardActionOpenDetail)),
	}, "\n")
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

func footerHelpText(active mode.ID, width int, keys config.ResolvedKeyBindings) string {
	if width < 90 {
		switch active {
		case mode.Search:
			return fmt.Sprintf("Search: type+enter %s %s/%s %s %s %s", keys.DisplayPrimary(config.SearchContext, config.SearchActionFocusQuery), keys.DisplayPrimary(config.SearchContext, config.SearchActionCycleFocusNext), keys.DisplayPrimary(config.SearchContext, config.SearchActionCycleFocusPrev), keys.DisplayPrimary(config.SearchContext, config.SearchActionMoveDown)+"/"+keys.DisplayPrimary(config.SearchContext, config.SearchActionMoveUp), keys.DisplayPrimary(config.SearchContext, config.SearchActionOpenDetail), keys.DisplayPrimary(config.ShellContext, config.ShellActionEscape))
		case mode.Detail:
			return fmt.Sprintf("Detail: %s/%s %s/%s %s/%s %s", keys.DisplayPrimary(config.DetailContext, config.DetailActionScrollDown), keys.DisplayPrimary(config.DetailContext, config.DetailActionScrollUp), keys.DisplayPrimary(config.DetailContext, config.DetailActionPageUp), keys.DisplayPrimary(config.DetailContext, config.DetailActionPageDown), keys.DisplayPrimary(config.DetailContext, config.DetailActionHome), keys.DisplayPrimary(config.DetailContext, config.DetailActionEnd), keys.DisplayPrimary(config.ShellContext, config.ShellActionEscape))
		default:
			return fmt.Sprintf("Board: %s %s %s %s %s %s", keys.DisplayPrimary(config.BoardContext, config.BoardActionMoveLeft)+"/"+keys.DisplayPrimary(config.BoardContext, config.BoardActionMoveRight), keys.DisplayPrimary(config.BoardContext, config.BoardActionMoveDown)+"/"+keys.DisplayPrimary(config.BoardContext, config.BoardActionMoveUp), keys.DisplayPrimary(config.BoardContext, config.BoardActionOpenDetail), keys.DisplayPrimary(config.ShellContext, config.ShellActionToggleSearch), keys.DisplayPrimary(config.ShellContext, config.ShellActionHelp), keys.DisplayPrimary(config.ShellContext, config.ShellActionQuit))
		}
	}

	switch active {
	case mode.Search:
		return fmt.Sprintf("Search: type + Enter query · %s focus · %s/%s switch panes · %s/%s query/results · %s detail · %s board", keys.DisplayPrimary(config.SearchContext, config.SearchActionFocusQuery), keys.DisplayPrimary(config.SearchContext, config.SearchActionFocusLeft), keys.DisplayPrimary(config.SearchContext, config.SearchActionFocusRight), keys.DisplayPrimary(config.SearchContext, config.SearchActionMoveDown), keys.DisplayPrimary(config.SearchContext, config.SearchActionMoveUp), keys.DisplayPrimary(config.SearchContext, config.SearchActionOpenDetail), keys.DisplayPrimary(config.ShellContext, config.ShellActionEscape))
	case mode.Detail:
		return fmt.Sprintf("Detail: %s/%s scroll · %s/%s page · %s/%s bounds · %s edit · %s back", keys.DisplayPrimary(config.DetailContext, config.DetailActionScrollDown), keys.DisplayPrimary(config.DetailContext, config.DetailActionScrollUp), keys.DisplayPrimary(config.DetailContext, config.DetailActionPageUp), keys.DisplayPrimary(config.DetailContext, config.DetailActionPageDown), keys.DisplayPrimary(config.DetailContext, config.DetailActionHome), keys.DisplayPrimary(config.DetailContext, config.DetailActionEnd), keys.DisplayPrimary(config.ShellContext, config.ShellActionEditIssue), keys.DisplayPrimary(config.ShellContext, config.ShellActionEscape))
	default:
		return fmt.Sprintf("Board: %s/%s columns · %s/%s issues · %s detail · %s search · %s help · %s quit", keys.DisplayPrimary(config.BoardContext, config.BoardActionMoveLeft), keys.DisplayPrimary(config.BoardContext, config.BoardActionMoveRight), keys.DisplayPrimary(config.BoardContext, config.BoardActionMoveDown), keys.DisplayPrimary(config.BoardContext, config.BoardActionMoveUp), keys.DisplayPrimary(config.BoardContext, config.BoardActionOpenDetail), keys.DisplayPrimary(config.ShellContext, config.ShellActionToggleSearch), keys.DisplayPrimary(config.ShellContext, config.ShellActionHelp), keys.DisplayPrimary(config.ShellContext, config.ShellActionQuit))
	}
}
