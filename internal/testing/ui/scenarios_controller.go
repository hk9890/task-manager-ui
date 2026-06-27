package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// InitializeController runs Init and resolves all emitted messages/commands.
func InitializeController(controller Controller) Controller {
	if controller == nil {
		return nil
	}

	return applyControllerCmd(controller, controller.Init())
}

// ApplyControllerKeySequence sends key messages and resolves resulting commands.
func ApplyControllerKeySequence(controller Controller, keys ...tea.KeyMsg) Controller {
	current := controller
	for _, key := range keys {
		cmd := current.Update(key)
		current = applyControllerCmd(current, cmd)
	}

	return current
}

func applyControllerCmd(controller Controller, cmd tea.Cmd) Controller {
	current := controller
	queue := drainCmd(cmd)
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]

		nested := current.Update(msg)
		queue = append(queue, drainCmd(nested)...)
	}

	return current
}
