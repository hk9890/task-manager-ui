package ui

import tea "github.com/charmbracelet/bubbletea"

// InitializeModel runs Init and resolves all emitted messages/commands.
func InitializeModel(model tea.Model) tea.Model {
	if model == nil {
		return nil
	}

	return applyCmd(model, model.Init())
}

// ApplyKeySequence sends key messages and resolves all resulting commands.
func ApplyKeySequence(model tea.Model, keys ...tea.KeyMsg) tea.Model {
	current := model
	for _, key := range keys {
		next, cmd := current.Update(key)
		current = applyCmd(next, cmd)
	}

	return current
}

// OpenDetailKeys returns the shell key sequence for opening detail mode.
func OpenDetailKeys() []tea.KeyMsg {
	return []tea.KeyMsg{{Type: tea.KeyEnter}}
}

// DetailBackKeys returns the shell key sequence for leaving detail mode.
func DetailBackKeys() []tea.KeyMsg {
	return []tea.KeyMsg{{Type: tea.KeyEsc}}
}

// DetailScrollKeys returns a representative deterministic detail scroll sequence.
func DetailScrollKeys() []tea.KeyMsg {
	return []tea.KeyMsg{{Type: tea.KeyPgDown}, {Type: tea.KeyEnd}}
}

func applyCmd(model tea.Model, cmd tea.Cmd) tea.Model {
	current := model
	queue := DrainCmd(cmd)
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]

		next, nested := current.Update(msg)
		current = next
		queue = append(queue, DrainCmd(nested)...)
	}

	return current
}

// DrainCmd runs cmd to completion and returns the flat list of non-batch
// messages it produced, expanding nested tea.BatchMsg values breadth-first.
//
// Exported because two mode test packages cannot reach an unexported helper and
// had each re-implemented the same queue loop.
func DrainCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	queue := []tea.Msg{cmd()}
	msgs := make([]tea.Msg, 0, len(queue))
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]

		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, nested := range batch {
				if nested != nil {
					queue = append(queue, nested())
				}
			}
			continue
		}

		msgs = append(msgs, msg)
	}

	return msgs
}
