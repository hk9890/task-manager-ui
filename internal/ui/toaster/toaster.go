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
}

// New creates a toaster model.
func New() Model {
	return Model{}
}

// Show displays a toast message.
func (m Model) Show(message string, style Style) Model {
	m.message = message
	m.style = style
	m.visible = true
	return m
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

// DismissMsg signals automatic dismissal.
type DismissMsg struct{}

// ScheduleDismiss emits DismissMsg after d.
func ScheduleDismiss(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(_ time.Time) tea.Msg {
		return DismissMsg{}
	})
}
