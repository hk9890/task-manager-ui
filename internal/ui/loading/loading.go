// Package loading provides shared loading-feedback primitives for the app shell.
package loading

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// spinnerFrameCount is the length of the frame counter's cycle. Every glyph
// set's spinner divides it, so no set jumps where the counter wraps.
const spinnerFrameCount = 10

// TickMsg is the message type fired by SpinnerTickCmd on each tick.
type TickMsg struct{}

// NextFrame returns the next spinner frame index after prev.
func NextFrame(prev int) int {
	return (prev + 1) % spinnerFrameCount
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
	frames := styles.Glyphs.Spinner
	n := len(frames)
	return frames[((frame%n)+n)%n]
}

// SpinnerTickCmd returns a tea.Cmd that fires a TickMsg after duration d.
func SpinnerTickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return TickMsg{} })
}
