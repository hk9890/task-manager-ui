package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/domain"
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

// launchActionCmd starts an external tool. The runner ignores ctx by design so
// launched processes outlive taskmgr-ui (see launcher.execProcessRunner.Run).
func launchActionCmd(ctx context.Context, services Services, action string, issue domain.IssueDetail) tea.Cmd {
	return func() tea.Msg {
		err := services.Launcher.Launch(ctx, action, issue)
		return launchActionResultMsg{action: action, err: err}
	}
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
