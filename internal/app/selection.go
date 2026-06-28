package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/mode/detail"
)

func (m Model) currentSelection() *mode.Selection {
	if m.lastBrowse != mode.Board && m.lastBrowse != mode.Search {
		if m.active == mode.Board || m.active == mode.Search {
			return m.selectedByMode[m.active]
		}
		return nil
	}

	if m.active == mode.Board || m.active == mode.Search {
		return m.selectedByMode[m.active]
	}

	return m.selectedByMode[m.lastBrowse]
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
	return loadDetailCmd(m.services, selection.Issue.ID)
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
