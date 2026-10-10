package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type scenarioCounterModel struct {
	count int
}

func (m scenarioCounterModel) Init() tea.Cmd {
	return func() tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")} }
}

func (m scenarioCounterModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		m.count += len(key.Runes)
	}
	return m, nil
}

func (m scenarioCounterModel) View() string {
	return ""
}

func TestScenarioHelpersProvideDeterministicKeyFlows(t *testing.T) {
	t.Parallel()

	if got := len(OpenDetailKeys()); got != 1 || OpenDetailKeys()[0].Type != tea.KeyEnter {
		t.Fatalf("expected open detail enter key, got %#v", OpenDetailKeys())
	}
	if got := len(DetailBackKeys()); got != 1 || DetailBackKeys()[0].Type != tea.KeyEsc {
		t.Fatalf("expected detail back esc key, got %#v", DetailBackKeys())
	}
	if got := DetailScrollKeys(); len(got) != 2 || got[0].Type != tea.KeyPgDown || got[1].Type != tea.KeyEnd {
		t.Fatalf("expected detail scroll sequence [pgdown end], got %#v", got)
	}
	model := InitializeModel(scenarioCounterModel{})
	model = ApplyKeySequence(model,
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")},
	)
	final, ok := model.(scenarioCounterModel)
	if !ok {
		t.Fatalf("expected scenarioCounterModel final type, got %T", model)
	}
	if final.count != 3 {
		t.Fatalf("expected init+typed rune count 3, got %d", final.count)
	}
}
