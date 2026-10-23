package app

import (
	"context"
	"time"

	"github.com/charmbracelet/colorprofile"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/revier"
	"github.com/hk9890/task-manager-ui/internal/storecatalog"
)

// refreshTickMsg triggers periodic surface auto-refresh.
type refreshTickMsg struct{}

// agentPollTickMsg asks revier for the live agents of the project again.
type agentPollTickMsg struct{}

// agentsListedMsg carries the live agents of the store's revier project.
// project is "" when the project root resolved to no revier project.
type agentsListedMsg struct {
	project string
	agents  []revier.Agent
	err     error
}

// agentFocusedMsg carries the result of bringing an agent to the front.
type agentFocusedMsg struct{ err error }

// storeChangedMsg reports that the active store changed on disk.
type storeChangedMsg struct{}

// storeWatchEndedMsg reports that the active store's change watch ended while
// the store was still open.
type storeWatchEndedMsg struct{}

// startupHealthCheckMsg carries the result of the startup repository health check.
type startupHealthCheckMsg struct{ err error }

// detailLoadedMsg carries the result of a detail load for a specific issue.
type detailLoadedMsg struct {
	issueID string
	detail  domain.IssueDetail
	err     error
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

	// loading is whether the surface was loading after the previous update, and
	// loadedAtChange the Model.storeChangeSeq its latest load started at. A
	// surface whose loadedAtChange is behind the sequence shows a store that
	// has changed since.
	loading        bool
	loadedAtChange int
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

	// ColorProfile is the colour profile of the terminal the app draws on.
	// The configuration screen applies a theme on it, because the hover roles
	// depend on it. A tea.ColorProfileMsg that reports another one replaces
	// it. The zero value, Unknown, is drawn as true colour.
	ColorProfile colorprofile.Profile

	// Ctx is the application lifecycle context. Each store the app opens gets
	// a context derived from it, and repository reads use that one, so both
	// quitting and switching stores abandon work in flight instead of waiting
	// for it. Nil means context.Background().
	Ctx context.Context
}
