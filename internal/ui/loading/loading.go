// Package loading provides shared loading-feedback primitives for the app shell.
package loading

import (
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// SpinnerFrames is the pinned braille spinner glyph sequence.
var SpinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// TickMsg is the message type fired by SpinnerTickCmd on each tick.
type TickMsg struct{}

// NextFrame returns the next spinner frame index after prev.
func NextFrame(prev int) int {
	return (prev + 1) % len(SpinnerFrames)
}

// SkeletonPhase returns the skeleton color-cycle index for a given spinner
// frame counter. Phase advances every 4 frames (~400 ms at the 100 ms spinner
// tick), giving a full 3-shade cycle every ~1.2 s.
// Negative frame values return 0 (no defined behavior for negative counts;
// callers must pass a non-negative spinnerFrame).
func SkeletonPhase(frame int) int {
	if frame < 0 {
		return 0
	}
	return frame / 4
}

// Glyph returns the spinner glyph string for the given frame index.
// Defensive against negative input.
func Glyph(frame int) string {
	n := len(SpinnerFrames)
	return string(SpinnerFrames[((frame%n)+n)%n])
}

// SpinnerTickCmd returns a tea.Cmd that fires a TickMsg after duration d.
func SpinnerTickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return TickMsg{} })
}

// Scope identifies which shell surface is loading.
type Scope string

const (
	ScopeBoard  Scope = "board"
	ScopeSearch Scope = "search"
	ScopeDetail Scope = "detail"
)

// State describes one loading state for shared rendering.
type State struct {
	Scope  Scope
	Target string
}

// Summary renders a shared footer/status-line summary for all active loading states.
func Summary(states []State) string {
	if len(states) == 0 {
		return lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render("Idle")
	}

	scopes := make([]string, 0, len(states))
	for _, state := range states {
		scope := strings.TrimSpace(string(state.Scope))
		if scope == "" {
			continue
		}
		if slices.Contains(scopes, scope) {
			continue
		}
		scopes = append(scopes, scope)
	}

	if len(scopes) == 0 {
		return lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render("Idle")
	}

	return lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render("Loading: " + strings.Join(scopes, ", "))
}
