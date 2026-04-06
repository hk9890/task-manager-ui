package details

import (
	"github.com/hk9890/beads-workbench/internal/domain"
	uidetails "github.com/hk9890/beads-workbench/internal/ui/details"
)

// Model is the shell-owned standalone detail presentation state.
type Model struct {
	SelectionID string
	TargetID    string
	Detail      domain.IssueDetail
	Loading     bool
	Error       string
}

// View renders the detail surface for pane and dedicated detail mode.
func (m Model) View(maxWidth int, compact bool) string {
	return uidetails.Render(uidetails.State{
		SelectionID: m.SelectionID,
		TargetID:    m.TargetID,
		Detail:      m.Detail,
		Loading:     m.Loading,
		Error:       m.Error,
		Width:       maxWidth,
		Compact:     compact,
	})
}
