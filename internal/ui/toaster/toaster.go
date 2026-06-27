// Package toaster provides small toast notifications for shell feedback.
package toaster

import (
	"time"

	"github.com/hk9890/task-manager-ui/internal/ui/overlay"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// View renders the toast box.
func (m Model) View() string {
	if !m.visible || m.message == "" {
		return ""
	}

	s := lipgloss.NewStyle().
		Padding(0, 1).
		Border(lipgloss.RoundedBorder())

	content := "✅ " + m.message
	switch m.style {
	case StyleError:
		s = s.BorderForeground(styles.ToastBorderErrorColor)
		content = "❌ " + m.message
	case StyleInfo:
		s = s.BorderForeground(styles.ToastBorderInfoColor)
		content = "ℹ️ " + m.message
	case StyleWarn:
		s = s.BorderForeground(styles.ToastBorderWarnColor)
		content = "⚠️ " + m.message
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
	}, m.View(), bg)
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
