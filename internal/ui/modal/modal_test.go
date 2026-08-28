package modal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
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

func TestPrevField(t *testing.T) {
	t.Parallel()

	twoInputCfg := Config{
		Title: "Edit",
		Inputs: []InputConfig{
			{Key: "first", Label: "First", Value: "a"},
			{Key: "second", Label: "Second", Value: "b"},
		},
	}

	t.Run("middle input: prev decrements focusedInput", func(t *testing.T) {
		t.Parallel()
		m := New(twoInputCfg)
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
		if m.FocusedInput() != 1 {
			t.Fatalf("setup: expected focusedInput=1, got %d", m.FocusedInput())
		}
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
		if m.FocusedInput() != 0 {
			t.Fatalf("expected focusedInput=0 after prev, got %d", m.FocusedInput())
		}
	})

	t.Run("first input: prev wraps to buttons (FieldCancel)", func(t *testing.T) {
		t.Parallel()
		m := New(twoInputCfg)
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
		if m.FocusedInput() != -1 {
			t.Fatalf("expected focusedInput=-1 after prev from first input, got %d", m.FocusedInput())
		}
		if m.FocusedField() != FieldCancel {
			t.Fatalf("expected focusedField=FieldCancel after prev from first input, got %v", m.FocusedField())
		}
	})

	t.Run("FieldCancel button: prev moves to FieldSave", func(t *testing.T) {
		t.Parallel()
		m := New(twoInputCfg)
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // 0→1
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // 1→Save
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Save→Cancel
		if m.FocusedInput() != -1 || m.FocusedField() != FieldCancel {
			t.Fatalf("setup: expected buttons/Cancel, got input=%d field=%v", m.FocusedInput(), m.FocusedField())
		}
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
		if m.FocusedInput() != -1 {
			t.Fatalf("expected to remain on buttons, got focusedInput=%d", m.FocusedInput())
		}
		if m.FocusedField() != FieldSave {
			t.Fatalf("expected focusedField=FieldSave after prev from Cancel, got %v", m.FocusedField())
		}
	})

	t.Run("FieldSave button with inputs: prev wraps to last input", func(t *testing.T) {
		t.Parallel()
		m := New(twoInputCfg)
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // 0→1
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // 1→Save
		if m.FocusedInput() != -1 || m.FocusedField() != FieldSave {
			t.Fatalf("setup: expected buttons/Save, got input=%d field=%v", m.FocusedInput(), m.FocusedField())
		}
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
		if m.FocusedInput() != 1 {
			t.Fatalf("expected focusedInput=1 (last input) after prev from Save, got %d", m.FocusedInput())
		}
	})

	t.Run("FieldSave button without inputs: prev wraps to FieldCancel", func(t *testing.T) {
		t.Parallel()
		m := New(Config{Title: "Confirm", Message: "Sure?"})
		if m.FocusedInput() != -1 || m.FocusedField() != FieldSave {
			t.Fatalf("setup: expected buttons/Save, got input=%d field=%v", m.FocusedInput(), m.FocusedField())
		}
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
		if m.FocusedInput() != -1 {
			t.Fatalf("expected to remain on buttons, got focusedInput=%d", m.FocusedInput())
		}
		if m.FocusedField() != FieldCancel {
			t.Fatalf("expected focusedField=FieldCancel after prev from Save (no inputs), got %v", m.FocusedField())
		}
	})
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

// TestEnterOnFocusedInputAdvancesInsteadOfSubmitting is the counterpart to the
// test above: with SubmitOnEnter off — the default, and what every mutation
// dialog uses — Enter walks the fields and only submits once focus has left the
// inputs.
//
// This is easy to "fix" in the helpful direction, and doing so would submit
// half-filled multi-field dialogs on the first Enter.
func TestEnterOnFocusedInputAdvancesInsteadOfSubmitting(t *testing.T) {
	t.Parallel()

	m := New(Config{
		Title: "Two fields",
		Inputs: []InputConfig{
			{Key: "first", Label: "First"},
			{Key: "second", Label: "Second"},
		},
	})

	if got := m.FocusedInput(); got != 0 {
		t.Fatalf("expected focus to start on the first input, got %d", got)
	}

	assertNotSubmit := func(cmd tea.Cmd, when string) {
		t.Helper()
		if cmd == nil {
			return
		}
		if _, isSubmit := cmd().(SubmitMsg); isSubmit {
			t.Fatalf("Enter %s must not submit", when)
		}
	}

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assertNotSubmit(cmd, "on the first input")
	if got := m.FocusedInput(); got != 1 {
		t.Fatalf("expected focus to advance to the second input, got %d", got)
	}

	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assertNotSubmit(cmd, "on the last input")
	if got := m.FocusedInput(); got != -1 {
		t.Fatalf("expected focus to leave the inputs, got %d", got)
	}
	if got := m.FocusedField(); got != FieldSave {
		t.Fatalf("expected focus to land on Save, got %v", got)
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected Enter on the Save button to submit")
	}
	if _, ok := cmd().(SubmitMsg); !ok {
		t.Fatalf("expected SubmitMsg from the Save button, got %T", cmd())
	}
}

// TestTypedRunesReachSubmitValues covers the path a user actually takes. The
// other submit tests seed InputConfig.Value and never press a rune key, so a
// break between key handling and values() would not show up in them.
func TestTypedRunesReachSubmitValues(t *testing.T) {
	t.Parallel()

	m := New(Config{
		Title:  "Comment",
		Inputs: []InputConfig{{Key: "body", Label: "Comment"}},
	})

	for _, r := range "looks good" {
		// Discard the cmd on purpose: textinput returns a cursor-blink tick and
		// invoking it here would block the test on a timer.
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected submit command")
	}

	submit, ok := cmd().(SubmitMsg)
	if !ok {
		t.Fatalf("expected SubmitMsg, got %T", cmd())
	}
	if submit.Values["body"] != "looks good" {
		t.Fatalf("typed text did not reach the submit values: %#v", submit.Values)
	}
}

// TestSubmitCmdDoesNotRaceWithFurtherTyping pins that the submit Cmd carries a
// snapshot rather than a view of the live model.
//
// Update has a value receiver, so a closure over m captured a Model whose
// inputs slice shared its backing array with the live one, and Bubble Tea runs
// every Cmd on its own goroutine. The shell clears the modal only when the
// SubmitMsg arrives, so a keystroke in that window assigned a whole
// textinput.Model into the shared array while the closure read it. Run under
// -race (mise run test does), this test reported that race before the fix.
func TestSubmitCmdDoesNotRaceWithFurtherTyping(t *testing.T) {
	t.Parallel()

	// SubmitOnEnter keeps the input focused across the submit, which is the
	// shape that races: every later keystroke writes into the same inputs
	// array the Cmd reads.
	m := New(Config{
		Title:         "Comment",
		Inputs:        []InputConfig{{Key: "body", Label: "Comment"}},
		SubmitOnEnter: true,
	})

	for _, r := range "ship it" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected submit command")
	}
	m = next

	// The Cmd runs on its own goroutine while the update loop keeps handling
	// keys, exactly as Bubble Tea would.
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()

	for _, r := range " now" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	submit, ok := (<-done).(SubmitMsg)
	if !ok {
		t.Fatal("expected SubmitMsg from the submit command")
	}
	if submit.Values["body"] != "ship it" {
		t.Fatalf("submitted values changed after Enter: got %q, want %q", submit.Values["body"], "ship it")
	}
}
