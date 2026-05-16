package modal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/beads-workbench/internal/config"
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

func TestSubmitAllowsPartialInputWhenNotRequired(t *testing.T) {
	m := New(Config{
		Title:    "Input",
		Required: false,
		Inputs: []InputConfig{
			{Key: "first", Label: "First", Value: "a"},
			{Key: "second", Label: "Second", Value: ""},
		},
	})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected submit command")
	}

	msg := cmd()
	submit, ok := msg.(SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", msg)
	}
	if submit.Values["first"] != "a" || submit.Values["second"] != "" {
		t.Fatalf("unexpected submit values: %#v", submit.Values)
	}
}

func TestBindingsFromConfigUsesConfiguredModalKeys(t *testing.T) {
	t.Parallel()

	resolved, err := config.ResolveKeyBindings(config.MergeKeyBindings(config.DefaultKeyBindings(), &config.KeyBindingOverride{
		Modal: map[string][]string{
			config.ModalActionNext:   {"ctrl+n"},
			config.ModalActionPrev:   {"ctrl+p"},
			config.ModalActionLeft:   {"a"},
			config.ModalActionRight:  {"d"},
			config.ModalActionEnter:  {"space"},
			config.ModalActionEscape: {"q"},
		},
	}))
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}

	m := NewWithKeys(Config{Title: "Confirm", Message: "Are you sure?"}, BindingsFromConfig(resolved))
	if m.FocusedInput() != -1 {
		t.Fatalf("expected button focus, got %d", m.FocusedInput())
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.FocusedField() != FieldCancel {
		t.Fatalf("expected configured right key to focus cancel, got %v", m.FocusedField())
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected configured escape key to emit cancel")
	}
	if _, ok := cmd().(CancelMsg); !ok {
		t.Fatalf("expected CancelMsg, got %T", cmd())
	}

	m = NewWithKeys(Config{
		Title:  "Input",
		Inputs: []InputConfig{{Key: "name", Label: "Name", Value: "alice"}},
	}, BindingsFromConfig(resolved))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if cmd == nil {
		t.Fatal("expected configured enter key to submit")
	}
	if _, ok := cmd().(SubmitMsg); !ok {
		t.Fatalf("expected SubmitMsg, got %T", cmd())
	}
}

func TestNKeyDoesNotCancelWhenRequired(t *testing.T) {
	m := New(Config{
		Title:    "Confirm",
		Message:  "Required confirmation",
		Required: true,
	})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if cmd != nil {
		if _, ok := cmd().(CancelMsg); ok {
			t.Fatalf("expected no CancelMsg for required modal when pressing n")
		}
	}
}

func TestEscapeCancelsEvenWhenRequired(t *testing.T) {
	m := New(Config{
		Title:    "Required",
		Required: true,
		Inputs: []InputConfig{{
			Key:   "status",
			Label: "Status",
			Value: "open",
		}},
	})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected escape to emit cancel command")
	}
	if _, ok := cmd().(CancelMsg); !ok {
		t.Fatalf("expected CancelMsg, got %T", cmd())
	}
}

func TestSubmitOnEnterSubmitsFromFocusedInput(t *testing.T) {
	m := New(Config{
		Title:         "Status",
		SubmitOnEnter: true,
		Required:      true,
		Inputs: []InputConfig{{
			Key:   "status",
			Label: "Status",
			Value: "open",
		}},
	})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected enter to submit from focused input")
	}

	msg := cmd()
	submit, ok := msg.(SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", msg)
	}
	if submit.Values["status"] != "open" {
		t.Fatalf("expected submitted status open, got %#v", submit.Values)
	}
}
