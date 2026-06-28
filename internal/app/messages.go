package app

import (
	"time"

	"github.com/hk9890/task-manager-ui/internal/domain"
	launchereditor "github.com/hk9890/task-manager-ui/internal/launcher/editor"
)

// refreshTickMsg triggers periodic surface auto-refresh.
type refreshTickMsg struct{}

// startupHealthCheckMsg carries the result of the startup repository health check.
type startupHealthCheckMsg struct{ err error }

// detailLoadedMsg carries the result of a detail load for a specific issue.
type detailLoadedMsg struct {
	issueID string
	detail  domain.IssueDetail
	err     error
}

// editIssueResultMsg carries the result of the full edit round-trip.
type editIssueResultMsg struct {
	issueID string
	updated bool
	err     error
}

// editIssuePreparedMsg carries the result of the PrepareDocument phase.
type editIssuePreparedMsg struct {
	issueID  string
	prepared launchereditor.Prepared
	err      error
}

// editorExitedMsg is delivered by the tea.Exec callback when the editor process exits.
type editorExitedMsg struct {
	prepared launchereditor.Prepared
	execErr  error
}

// launchActionResultMsg carries the result of a background launcher action.
type launchActionResultMsg struct {
	action string
	err    error
}

// surfaceRefreshState tracks dirty and last-refresh time for a browse surface.
type surfaceRefreshState struct {
	dirty       bool
	lastRefresh time.Time
}

// RuntimeOptions carries toggles that alter runtime behaviour without touching config.
type RuntimeOptions struct {
	DisableAutoRefresh bool
}
