package app

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
	launchereditor "github.com/hk9890/task-manager-ui/internal/launcher/editor"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

func loadDetailCmd(services Services, issueID string) tea.Cmd {
	return func() tea.Msg {
		detail, err := services.Repo.Issue(context.Background(), issueID)
		return detailLoadedMsg{issueID: issueID, detail: detail, err: err}
	}
}

// prepareEditCmd runs the PrepareDocument phase in a goroutine. The result is
// delivered as editIssuePreparedMsg; the model then returns tea.Exec to hand
// terminal control to the editor process.
func prepareEditCmd(services Services, issueID string) tea.Cmd {
	return func() tea.Msg {
		prepared, err := services.Editor.PrepareDocument(context.Background(), issueID)
		return editIssuePreparedMsg{issueID: issueID, prepared: prepared, err: err}
	}
}

// applyEditsCmd runs the ApplyEdits phase in a goroutine after the editor exits
// cleanly (execErr == nil path). It reads the temp file, parses the document,
// and calls UpdateIssue if there are changes. Temp-file cleanup is handled
// inside ApplyEdits. On editor exec error the caller short-circuits before
// reaching here, so no UpdateIssue call is possible from an error path.
func applyEditsCmd(services Services, prepared launchereditor.Prepared) tea.Cmd {
	return func() tea.Msg {
		result, err := services.Editor.ApplyEdits(context.Background(), prepared.IssueID, prepared.Issue, prepared.TempPath)
		if err != nil {
			return editIssueResultMsg{issueID: prepared.IssueID, err: err}
		}
		return editIssueResultMsg{issueID: prepared.IssueID, updated: result.Updated}
	}
}

func launchActionCmd(services Services, action string, issue domain.IssueDetail) tea.Cmd {
	return func() tea.Msg {
		err := services.Launcher.Launch(context.Background(), action, issue)
		return launchActionResultMsg{action: action, err: err}
	}
}

// handleEditIssuePrepared processes editIssuePreparedMsg in Update.
func (m Model) handleEditIssuePrepared(modeCmd tea.Cmd, msg editIssuePreparedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, batchCmds(modeCmd, m.showToast(fmt.Sprintf("Failed to edit issue %s", msg.issueID), toaster.StyleError))
	}
	editorCmd, err := m.services.Editor.BuildEditorCmd(msg.prepared.TempPath)
	if err != nil {
		// PrepareDocument already wrote the temp doc (containing the issue's
		// title + description); remove it on this error path so it does not leak
		// on disk until the stale-temp sweep — matching the editorExitedMsg and
		// ApplyEdits cleanup paths.
		_ = os.Remove(msg.prepared.TempPath)
		return m, batchCmds(modeCmd, m.showToast(fmt.Sprintf("Failed to build editor command: %v", err), toaster.StyleError))
	}
	prepared := msg.prepared
	execCommand := m.services.ExecCommandFactory(editorCmd)
	return m, batchCmds(modeCmd, tea.Exec(execCommand, func(err error) tea.Msg {
		return editorExitedMsg{prepared: prepared, execErr: err}
	}))
}

// handleEditorExited processes editorExitedMsg in Update.
func (m Model) handleEditorExited(modeCmd tea.Cmd, msg editorExitedMsg) (tea.Model, tea.Cmd) {
	if msg.execErr != nil {
		_ = os.Remove(msg.prepared.TempPath)
		issueID := msg.prepared.IssueID
		execErr := msg.execErr
		return m, batchCmds(modeCmd, func() tea.Msg {
			return editIssueResultMsg{issueID: issueID, err: fmt.Errorf("editor exited with error: %w", execErr)}
		})
	}
	return m, batchCmds(modeCmd, applyEditsCmd(m.services, msg.prepared))
}

// handleEditIssueResult processes editIssueResultMsg in Update.
func (m Model) handleEditIssueResult(modeCmd tea.Cmd, msg editIssueResultMsg) (tea.Model, tea.Cmd) {
	// notifyEditResult fires the test-only hook (if set) after the toast has
	// been set by showToast. Callers must call this before every return.
	notifyEditResult := func() {
		if h := m.onEditIssueResult; h != nil {
			h()
		}
	}

	if msg.err != nil {
		toastCmd := m.showToast(fmt.Sprintf("Failed to edit issue %s", msg.issueID), toaster.StyleError)
		notifyEditResult()
		return m, batchCmds(modeCmd, toastCmd)
	}

	if !msg.updated {
		toastCmd := m.showToast(fmt.Sprintf("No changes saved for issue %s", msg.issueID), toaster.StyleInfo)
		notifyEditResult()
		return m, batchCmds(modeCmd, toastCmd)
	}

	m.markBrowseSurfacesDirty()

	selection := m.currentSelection()
	if selection == nil || selection.Issue.ID == "" {
		toastCmd := m.showToast(fmt.Sprintf("Updated issue %s", msg.issueID), toaster.StyleSuccess)
		notifyEditResult()
		return m, batchCmds(modeCmd, toastCmd)
	}

	m.detail.SelectionID = selection.Issue.ID
	m.detail.SelectBrowserIssue(selection.Issue.ID)
	m.detail.Loading = true
	m.detail.Error = ""
	m.detail.TargetID = selection.Issue.ID
	toastCmd := m.showToast(fmt.Sprintf("Updated issue %s", msg.issueID), toaster.StyleSuccess)
	notifyEditResult()
	return m, batchCmds(modeCmd,
		toastCmd,
		loadDetailCmd(m.services, selection.Issue.ID),
	)
}

func batchCmds(cmds ...tea.Cmd) tea.Cmd {
	filtered := make([]tea.Cmd, 0, len(cmds))
	for _, cmd := range cmds {
		if cmd != nil {
			filtered = append(filtered, cmd)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return tea.Batch(filtered...)
}
