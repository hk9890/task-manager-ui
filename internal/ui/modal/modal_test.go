package modal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewConfirmMode(t *testing.T) {
	m := New(Config{Title: "Confirm", Message: "Are you sure?"})
	if m.hasInputs {
		t.Fatalf("confirm mode should have no inputs")
	}
	if m.FocusedInput() != -1 {
		t.Fatalf("confirm mode should start on buttons")
	}
}

func TestSubmitWithInput(t *testing.T) {
	m := New(Config{
		Title: "Input",
		Inputs: []InputConfig{{
			Key:   "name",
			Label: "Name",
			Value: "alice",
		}},
	})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // move to Save
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected submit command")
	}

	msg := cmd()
	submit, ok := msg.(SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", msg)
	}
	if submit.Values["name"] != "alice" {
		t.Fatalf("unexpected submit value")
	}
}
