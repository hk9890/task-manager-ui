package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Controller is the minimal concrete-mode contract used by UI tests.
// View takes a skeletonPhase int (pass 0 for tests that do not exercise
// the color-cycle animation).
type Controller interface {
	Init() tea.Cmd
	Update(tea.Msg) tea.Cmd
	View(skeletonPhase int) string
}

// ControllerAdapter wraps a concrete mode controller for teatest harnesses.
// It implements tea.Model by calling View(0) so tests always render at
// skeleton phase 0 (static, same appearance as before the pulse feature).
type ControllerAdapter struct {
	Controller Controller
}

func (a ControllerAdapter) Init() tea.Cmd {
	return a.Controller.Init()
}

func (a ControllerAdapter) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return a, a.Controller.Update(msg)
}

func (a ControllerAdapter) View() string {
	return a.Controller.View(0)
}
