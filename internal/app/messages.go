package app

import (
	"context"
	"time"

	"github.com/hk9890/task-manager-ui/internal/domain"
	launchereditor "github.com/hk9890/task-manager-ui/internal/launcher/editor"
	"github.com/hk9890/task-manager-ui/internal/storecatalog"
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

// unresolvedStoreMsg reports, once the program runs, why it started without a
// store.
type unresolvedStoreMsg struct{ reason string }

// storeOpenedMsg carries the result of opening a store the picker asked for.
type storeOpenedMsg struct {
	name   string
	opened storecatalog.Opened
	err    error
}

// storeCreatedMsg carries the result of creating a store from the picker.
type storeCreatedMsg struct {
	dir    string
	opened storecatalog.Opened
	err    error
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

	// UnresolvedStore is why startup found no store to open. Non-empty starts
	// the app on the store picker instead of a board, holds the operator there
	// until a store is open, and says why in a toast.
	UnresolvedStore string

	// StorelessDir is the working directory when no store resolved for it. The
	// picker offers to create a store there. Empty offers nothing — in
	// particular when --store-name named an unregistered store, since the
	// working directory may well have one of its own.
	StorelessDir string

	// Ctx is the application lifecycle context. Each store the app opens gets
	// a context derived from it, and repository reads use that one, so both
	// quitting and switching stores abandon work in flight instead of waiting
	// for it. Nil means context.Background().
	Ctx context.Context
}
