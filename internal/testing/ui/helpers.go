package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Controller is the minimal concrete-mode contract used by UI tests.
type Controller interface {
	Init() tea.Cmd
	Update(tea.Msg) tea.Cmd
	View() string
}

// ControllerAdapter wraps a concrete mode controller for teatest harnesses.
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
	return a.Controller.View()
}
