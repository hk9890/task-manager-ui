package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/mode/detail"
)

func (m Model) currentSelection() *mode.Selection {
	// A drill-in owns the selection for as long as Detail is showing it. Every
	// shell mutation and launcher reads this, so falling through to the browse
	// row here means `e` opens the board's issue while the screen shows the
	// child that was drilled into.
	if m.active == mode.Detail && m.drillSelection != nil {
		return m.drillSelection
	}
	// The active browse tab owns the selection; from Detail (or any other
	// non-browse mode) it comes from the tab we drilled in from.
	if mode.IsBrowse(m.active) {
		return m.selectedByMode[m.active]
	}
	if mode.IsBrowse(m.lastBrowse) {
		return m.selectedByMode[m.lastBrowse]
	}
	return nil
}

// clearDrillSelection drops a drill-in. Call it wherever the shell leaves
// Detail or a browse tab moves its own selection: from then on the browse row
// is what the operator is acting on again.
func (m *Model) clearDrillSelection() {
	m.drillSelection = nil
}

// enterBrowseMode switches to a browse tab, keeping lastBrowse in step and
// dropping any drill-in.
func (m *Model) enterBrowseMode(id mode.ID) {
	m.active = id
	m.lastBrowse = id
	m.clearDrillSelection()
}

func (m *Model) ensureDetailForCurrentSelectionCmd() tea.Cmd {
	selection := m.currentSelection()
	if selection == nil || selection.Issue.ID == "" {
		if m.active == mode.Detail {
			m.detail.Reset()
		}
		return nil
	}

	// Anchor above the guards. Both of them skip BeginLoad, which is where the
	// selection id and the rail anchor are written, so without this a re-fired
	// selection for the row already on screen leaves the rail highlighting
	// whichever dependency row the operator last moved the cursor to.
	m.detail.AnchorSelection(selection.Issue.ID)

	if m.detail.IsLoading() && m.detail.TargetID() == selection.Issue.ID {
		return nil
	}
	if !m.detail.IsLoading() && m.detail.Detail.Summary.ID == selection.Issue.ID && m.detail.Error() == "" && !m.shouldRefreshSurface(mode.Detail) {
		return nil
	}

	// Ref seeds an optimistic placeholder when the target issue changes, so
	// scroll offsets reset immediately rather than on the ShowIssue response.
	m.detail.BeginLoad(selection.Issue.ID, detail.BeginLoadOptions{
		Ref: &domain.IssueReference{
			ID:       selection.Issue.ID,
			Title:    selection.Issue.Title,
			Status:   selection.Issue.Status,
			Type:     selection.Issue.Type,
			Priority: selection.Issue.Priority,
		},
	})
	return m.loadDetail(selection.Issue.ID)
}

// drillInto navigates Detail to a related issue, from Enter or a double click
// on a row of its Dependencies rail.
//
// Drilling into a related issue is a full navigation, not a peek: the target
// becomes the new detail selection so ALL three panes — including the
// Dependencies rail — reflect the target once loaded. This is what lets you
// open a child from an epic and then jump back via the child's own Parent row.
// Seeding an optimistic placeholder from the row's known ref renders the header
// + core metadata immediately, while the description and Dependencies pane show
// their skeleton until the detail load returns. ApplyLoadedDetail
// resets scroll offsets when the issue changes.
//
// Focus retention: BeginLoad sets Loading and the drill-focus counter before
// the placeholder ApplyLoadedDetail call so that clearBrowserPanel does not flip
// focus away from the Dependencies pane during the in-flight window. The real
// detailLoadedMsg will apply the correct focus decision from actual rail content
// via the counter mechanism in ApplyLoadedDetail.
func (m *Model) drillInto(ref domain.IssueReference) tea.Cmd {
	issueID := strings.TrimSpace(ref.ID)
	if issueID == "" {
		return nil
	}
	m.active = mode.Detail
	m.drillSelection = &mode.Selection{Issue: domain.IssueSummary{
		ID:       issueID,
		Title:    ref.Title,
		Status:   ref.Status,
		Type:     ref.Type,
		Priority: ref.Priority,
	}}
	m.detail.BeginLoad(issueID, detail.BeginLoadOptions{Ref: &ref, Drill: true})
	return m.loadDetail(issueID)
}

// switchToTab makes a browse tab the active surface, from its mode key or a
// click on the header strip.
func (m *Model) switchToTab(id mode.ID) tea.Cmd {
	m.enterBrowseMode(id)
	return batchCmds(m.lazyInitActiveTabCmd(), m.ensureDetailForCurrentSelectionCmd(), m.maybeAutoRefreshActiveSurfaceCmd())
}

func (m Model) selectedIssueID() (string, bool) {
	selection := m.currentSelection()
	if selection == nil || selection.Issue.ID == "" {
		return "", false
	}

	return selection.Issue.ID, true
}

func (m Model) selectedIssueContext() (domain.IssueDetail, bool) {
	selection := m.currentSelection()
	if selection == nil || selection.Issue.ID == "" {
		return domain.IssueDetail{}, false
	}

	if m.detail.Detail.Summary.ID == selection.Issue.ID {
		return m.detail.Detail, true
	}

	return domain.IssueDetail{Summary: selection.Issue}, true
}
