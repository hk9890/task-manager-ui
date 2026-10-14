// Package toaster provides small toast notifications for shell feedback.
package toaster

import (
	"strings"
	"time"

	"github.com/hk9890/task-manager-ui/internal/ui/overlay"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Style determines toast appearance.
type Style int

const (
	StyleSuccess Style = iota
	StyleError
	StyleInfo
	StyleWarn
)

// Model holds toast state.
type Model struct {
	message string
	style   Style
	visible bool
	// seq is a monotonically increasing identity bumped on every Show. A
	// scheduled DismissMsg carries the seq of the toast it was scheduled for, so
	// a stale dismiss timer (from an earlier toast) cannot hide a newer toast
	// that replaced it within the dismiss window.
	seq int
}

// New creates a toaster model.
func New() Model {
	return Model{}
}

// Show displays a toast message. Each Show bumps the toast identity (seq) so a
// previously scheduled dismiss no longer matches the current toast.
func (m Model) Show(message string, style Style) Model {
	m.message = message
	m.style = style
	m.visible = true
	m.seq++
	return m
}

// Seq returns the current toast identity. Callers schedule a DismissMsg with
// this value and compare it on receipt to avoid dismissing a newer toast.
func (m Model) Seq() int {
	return m.seq
}

// Hide dismisses the toast.
func (m Model) Hide() Model {
	m.visible = false
	m.message = ""
	return m
}

// Visible reports whether the toast is visible.
func (m Model) Visible() bool {
	return m.visible
}

// toastChromeWidth is what the box spends on itself: one border cell and one
// padding cell on each side. glyphCells is the severity glyph plus its trailing
// space — ✅ and ❌ measure two cells and ℹ and ⚠ one, so budgeting the widest
// keeps every style inside the terminal.
const (
	toastChromeWidth = 4
	glyphCells       = 3
)

// maxMessageLines is the most lines a toast draws, the cut mark included. A message
// carries text from outside — an error, a path — and each line feed in it is a
// line of the box, so without a bound one message can cover the screen.
const maxMessageLines = 6

// View renders the toast box at its natural width. Overlay is what the shell
// draws with, and it bounds the box to the terminal.
func (m Model) View() string {
	return m.view(0)
}

// view renders the toast box, clipping each message line so the rendered box is
// never wider than maxWidth cells. maxWidth <= 0 leaves it unbounded.
//
// Nothing used to bound it. overlay.Place splices the foreground line into the
// background unclipped, so a message longer than the terminal — an error toast
// carries the repository's own text, and a launcher or editor failure names a
// path — made the terminal soft-wrap the box: the right border went missing and
// the wrapped remainder pushed the rest of the frame down.
func (m Model) view(maxWidth int) string {
	if !m.visible || m.message == "" {
		return ""
	}

	message := capLines(m.message)
	if maxWidth > toastChromeWidth {
		message = clipLines(message, maxWidth-toastChromeWidth-glyphCells)
	}

	s := lipgloss.NewStyle().
		Padding(0, 1).
		Border(lipgloss.RoundedBorder())

	content := styles.Glyphs.ToastSuccess + " " + message
	switch m.style {
	case StyleError:
		s = s.BorderForeground(styles.ToastBorderErrorColor)
		content = styles.Glyphs.ToastError + " " + message
	case StyleInfo:
		s = s.BorderForeground(styles.ToastBorderInfoColor)
		content = styles.Glyphs.ToastInfo + " " + message
	case StyleWarn:
		s = s.BorderForeground(styles.ToastBorderWarnColor)
		content = styles.Glyphs.ToastWarn + " " + message
	default:
		s = s.BorderForeground(styles.ToastBorderSuccessColor)
	}

	return s.Render(content)
}

// Overlay places the toast over a background using bottom-center placement.
func (m Model) Overlay(bg string, width, height int) string {
	if !m.visible || m.message == "" {
		return bg
	}

	return overlay.Place(overlay.Config{
		Width:    width,
		Height:   height,
		Position: overlay.Bottom,
		PadY:     1,
	}, m.view(width), bg)
}

// capLines bounds message to maxMessageLines lines. A longer message keeps one
// line fewer and ends with a mark on a line of its own, so a whole last line
// is not read as the end and the mark costs no row.
func capLines(message string) string {
	lines := strings.SplitN(message, "\n", maxMessageLines+1)
	if len(lines) <= maxMessageLines {
		return message
	}
	lines = lines[:maxMessageLines]
	lines[maxMessageLines-1] = "…"
	return strings.Join(lines, "\n")
}

// clipLines truncates every line of message to width cells. A message is
// usually one line; a repository error can carry several, and a box is only as
// narrow as its widest line.
func clipLines(message string, width int) string {
	if width < 1 {
		width = 1
	}
	lines := strings.Split(message, "\n")
	for i, line := range lines {
		lines[i] = textutil.TruncateString(line, width)
	}
	return strings.Join(lines, "\n")
}

// DismissMsg signals automatic dismissal of the toast identified by Seq. The
// shell hides the toast only when Seq matches the currently shown toast, so a
// stale timer from a superseded toast is ignored.
type DismissMsg struct {
	Seq int
}

// ScheduleDismiss emits DismissMsg after d, tagged with the toast identity seq
// so the receiver can ignore it if a newer toast has since been shown.
func ScheduleDismiss(d time.Duration, seq int) tea.Cmd {
	return tea.Tick(d, func(_ time.Time) tea.Msg {
		return DismissMsg{Seq: seq}
	})
}
