package app

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/ui/loading"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

const (
	defaultViewportWidth  = 120
	defaultViewportHeight = 34
	refreshTickInterval   = 60 * time.Second
	// agentPollInterval is how often revier is asked for the live agents: an
	// agent's status changes within seconds.
	agentPollInterval = 3 * time.Second
	// projectRetryTicks is the agent poll ticks a failed project lookup waits
	// before it runs again: about a minute.
	projectRetryTicks = int(time.Minute / agentPollInterval)
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

// defaultScheduleAgentPoll is the production agent poll timer. Stored
// per-Model for the same reason as defaultScheduleRefreshTick.
func defaultScheduleAgentPoll() tea.Cmd {
	return tea.Tick(agentPollInterval, func(_ time.Time) tea.Msg {
		return agentPollTickMsg{}
	})
}

// defaultAwaitStoreChange is the production wait for the next signal on a
// store's change channel. It blocks until the store changes or the channel
// closes, so it is stored per-Model like the schedulers: a test that drains
// commands replaces it.
func defaultAwaitStoreChange(changes <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		if _, open := <-changes; !open {
			return storeWatchEndedMsg{}
		}
		return storeChangedMsg{}
	}
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
