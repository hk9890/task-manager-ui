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

// BoardToSearchKeys returns the shell key sequence for board→search.
func BoardToSearchKeys() []tea.KeyMsg {
	return []tea.KeyMsg{{Type: tea.KeyCtrlAt}}
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

// SearchFocusResultsKeys returns key sequence that moves search focus to results.
func SearchFocusResultsKeys() []tea.KeyMsg {
	return []tea.KeyMsg{{Type: tea.KeyRight}}
}

// SearchClearQueryKeys returns the key sequence used to clear query quickly.
func SearchClearQueryKeys() []tea.KeyMsg {
	return []tea.KeyMsg{{Type: tea.KeyCtrlU}}
}

// SearchFragileQueryRunes returns historically fragile query-entry runes.
func SearchFragileQueryRunes() string {
	return "jkhlr"
}

// SearchTypeTextKeys returns key sequence for typing text into search query.
func SearchTypeTextKeys(text string) []tea.KeyMsg {
	keys := make([]tea.KeyMsg, 0, len([]rune(text)))
	for _, r := range []rune(text) {
		keys = append(keys, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return keys
}

func applyCmd(model tea.Model, cmd tea.Cmd) tea.Model {
	current := model
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

func drainCmd(cmd tea.Cmd) []tea.Msg {
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
