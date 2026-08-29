package app

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
	launchereditor "github.com/hk9890/task-manager-ui/internal/launcher/editor"
	"github.com/hk9890/task-manager-ui/internal/mode/detail"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

// Reads take the lifecycle ctx so quitting abandons them; writes deliberately
// do not. A cancelled read costs a discarded result, but a cancelled write can
// land half-applied, and the user quitting is not a request to undo a save.

func loadDetailCmd(ctx context.Context, services Services, issueID string) tea.Cmd {
	return func() tea.Msg {
		detail, err := services.Repo.Issue(ctx, issueID)
		return detailLoadedMsg{issueID: issueID, detail: detail, err: err}
	}
}

// prepareEditCmd runs the PrepareDocument phase in a goroutine. The result is
// delivered as editIssuePreparedMsg; the model then returns tea.Exec to hand
// terminal control to the editor process.
func prepareEditCmd(ctx context.Context, services Services, issueID string) tea.Cmd {
	return func() tea.Msg {
		prepared, err := services.Editor.PrepareDocument(ctx, issueID)
		return editIssuePreparedMsg{issueID: issueID, prepared: prepared, err: err}
	}
}

// applyEditsCmd runs the ApplyEdits phase in a goroutine after the editor exits
// cleanly (execErr == nil path). It reads the temp file, parses the document,
// and calls UpdateIssue if there are changes. Temp-file cleanup is handled
// inside ApplyEdits. On editor exec error the caller short-circuits before
// reaching here, so no UpdateIssue call is possible from an error path.
//
// Uncancellable on purpose: this is the write that persists what the user just
// typed into their editor.
func applyEditsCmd(services Services, prepared launchereditor.Prepared) tea.Cmd {
	return func() tea.Msg {
		result, err := services.Editor.ApplyEdits(context.Background(), prepared.IssueID, prepared.Issue, prepared.TempPath)
		if err != nil {
			return editIssueResultMsg{issueID: prepared.IssueID, err: err}
		}
		return editIssueResultMsg{issueID: prepared.IssueID, updated: result.Updated}
	}
}

// launchActionCmd starts an external tool. The runner ignores ctx by design so
// launched processes outlive taskmgr-ui (see launcher.execProcessRunner.Run).
func launchActionCmd(ctx context.Context, services Services, action string, issue domain.IssueDetail) tea.Cmd {
	return func() tea.Msg {
		err := services.Launcher.Launch(ctx, action, issue)
		return launchActionResultMsg{action: action, err: err}
	}
}

// handleEditIssuePrepared processes editIssuePreparedMsg in Update.
func (m Model) handleEditIssuePrepared(modeCmd tea.Cmd, msg editIssuePreparedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.logger().Error("failed to prepare the issue edit document", "issue_id", msg.issueID, "error", msg.err.Error())
		return m, batchCmds(modeCmd, m.showToast(fmt.Sprintf("Failed to edit issue %s: %v", msg.issueID, msg.err), toaster.StyleError))
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
		// Name the cause. ApplyEdits removes the temp document in a defer
		// before returning, so a bare "Failed to edit issue X" left the
		// operator with their rewritten text gone and no diagnostic anywhere —
		// not the store's own actionable messages ("issue is closed; reopen it
		// before editing"), not a parse error ("invalid priority \"P5\""), and
		// nothing in the log under --debug either.
		m.logger().Error("failed to apply the issue edit", "issue_id", msg.issueID, "error", msg.err.Error())
		toastCmd := m.showToast(fmt.Sprintf("Failed to edit issue %s: %v", msg.issueID, msg.err), toaster.StyleError)
		notifyEditResult()
		return m, batchCmds(modeCmd, toastCmd)
	}

	if !msg.updated {
		toastCmd := m.showToast(fmt.Sprintf("No changes saved for issue %s", msg.issueID), toaster.StyleInfo)
		notifyEditResult()
		return m, batchCmds(modeCmd, toastCmd)
	}

	// Marking the surfaces dirty only makes the *next* refresh tick reload
	// them, and that tick is a minute away — so the edited row kept its old
	// title under a toast saying the update succeeded, which reads as a failed
	// edit. handleMutationResult pairs the two calls for the same reason.
	m.markBrowseSurfacesDirty()

	selection := m.currentSelection()
	if selection == nil || selection.Issue.ID == "" {
		toastCmd := m.showToast(fmt.Sprintf("Updated issue %s", msg.issueID), toaster.StyleSuccess)
		notifyEditResult()
		return m, batchCmds(modeCmd, toastCmd, m.maybeAutoRefreshActiveSurfaceCmd())
	}

	m.detail.BeginLoad(selection.Issue.ID, detail.BeginLoadOptions{})
	toastCmd := m.showToast(fmt.Sprintf("Updated issue %s", msg.issueID), toaster.StyleSuccess)
	notifyEditResult()
	// After BeginLoad, so the detail-is-loading guard in refreshActiveSurfaceCmd
	// suppresses a second load of the detail this path already issues.
	return m, batchCmds(modeCmd,
		toastCmd,
		loadDetailCmd(m.ctx, m.services, selection.Issue.ID),
		m.maybeAutoRefreshActiveSurfaceCmd(),
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
