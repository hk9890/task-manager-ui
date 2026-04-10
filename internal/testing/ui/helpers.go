package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/beads-workbench/internal/mode"
)

// ControllerAdapter wraps a mode.Controller for teatest harnesses.
type ControllerAdapter struct {
	Controller mode.Controller
}

func (a ControllerAdapter) Init() tea.Cmd {
	return a.Controller.Init()
}

func (a ControllerAdapter) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := a.Controller.Update(msg)
	return ControllerAdapter{Controller: next}, cmd
}

func (a ControllerAdapter) View() string {
	return a.Controller.View()
}
