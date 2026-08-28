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
			m.detail = detail.Model{}
		}
		return nil
	}

	m.detail.SelectionID = selection.Issue.ID
	m.detail.SelectBrowserIssue(selection.Issue.ID)

	if m.detail.Loading && m.detail.TargetID == selection.Issue.ID {
		return nil
	}
	if !m.detail.Loading && m.detail.Detail.Summary.ID == selection.Issue.ID && m.detail.Error == "" && !m.shouldRefreshSurface(mode.Detail) {
		return nil
	}

	// When the target issue changes (new selection, not just a refresh of the
	// same issue), synchronously apply a placeholder detail BEFORE issuing the
	// repository call so that scroll offsets reset immediately rather than waiting
	// for the ShowIssue response.
	previousID := strings.TrimSpace(m.detail.Detail.Summary.ID)
	newID := selection.Issue.ID
	if previousID != strings.TrimSpace(newID) {
		// A board/search selection change supersedes any pending drill-focus sequence.
		m.detail.ClearDrillFocus()
		ref := domain.IssueReference{
			ID:       selection.Issue.ID,
			Title:    selection.Issue.Title,
			Status:   selection.Issue.Status,
			Type:     selection.Issue.Type,
			Priority: selection.Issue.Priority,
		}
		m.detail.ApplyLoadedDetail(newID, detail.PlaceholderDetail(newID, ref, true))
	}

	// Required: loadingStates() reads m.detail.Loading to drive the header spinner — do not remove.
	m.detail.Loading = true
	m.detail.Error = ""
	m.detail.TargetID = selection.Issue.ID
	return loadDetailCmd(m.ctx, m.services, selection.Issue.ID)
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
