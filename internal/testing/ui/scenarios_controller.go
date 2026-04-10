package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/beads-workbench/internal/mode"
)

// InitializeController runs Init and resolves all emitted messages/commands.
func InitializeController(controller mode.Controller) mode.Controller {
	if controller == nil {
		return nil
	}

	return applyControllerCmd(controller, controller.Init())
}

// ApplyControllerKeySequence sends key messages and resolves resulting commands.
func ApplyControllerKeySequence(controller mode.Controller, keys ...tea.KeyMsg) mode.Controller {
	current := controller
	for _, key := range keys {
		next, cmd := current.Update(key)
		current = applyControllerCmd(next, cmd)
	}

	return current
}

func applyControllerCmd(controller mode.Controller, cmd tea.Cmd) mode.Controller {
	current := controller
	queue := drainCmd(cmd)
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]

		next, nested := current.Update(msg)
		current = next
		queue = append(queue, drainCmd(nested)...)
	}

	return current
}
