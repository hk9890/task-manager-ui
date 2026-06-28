package app

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/ui/loading"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

const (
	defaultViewportWidth  = 120
	defaultViewportHeight = 34
	refreshTickInterval   = 60 * time.Second
)

// modelNow is the package-level clock used by refresh-state computations. Tests
// can replace it to produce deterministic time values.
var modelNow = time.Now

// defaultScheduleRefreshTick is the production implementation of the refresh
// tick scheduler. It is stored per-Model so tests can override it without a
// global mutex. Production code sets this once in NewModelWithOptions and
// never writes it again.
func defaultScheduleRefreshTick() tea.Cmd {
	return tea.Tick(refreshTickInterval, func(_ time.Time) tea.Msg {
		return refreshTickMsg{}
	})
}

// defaultScheduleToastDismiss is the production implementation of the toast
// dismiss scheduler. Stored per-Model for the same reason as
// defaultScheduleRefreshTick.
func defaultScheduleToastDismiss(d time.Duration, seq int) tea.Cmd {
	return toaster.ScheduleDismiss(d, seq)
}

// defaultScheduleSpinnerTick is the production implementation of the spinner
// tick scheduler. Stored per-Model for the same reason as
// defaultScheduleRefreshTick.
func defaultScheduleSpinnerTick() tea.Cmd {
	return loading.SpinnerTickCmd(100 * time.Millisecond)
}
