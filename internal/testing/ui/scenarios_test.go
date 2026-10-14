package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

type scenarioCounterModel struct {
	count int
}

func (m scenarioCounterModel) Init() tea.Cmd {
	return func() tea.Msg { return Runes("i") }
}

func (m scenarioCounterModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		m.count += len(key.String())
	}
	return m, nil
}

func (m scenarioCounterModel) View() tea.View {
	return tea.NewView("")
}

func TestScenarioHelpersProvideDeterministicKeyFlows(t *testing.T) {
	t.Parallel()

	if got := len(OpenDetailKeys()); got != 1 || OpenDetailKeys()[0].String() != "enter" {
		t.Fatalf("expected open detail enter key, got %#v", OpenDetailKeys())
	}
	if got := len(DetailBackKeys()); got != 1 || DetailBackKeys()[0].String() != "esc" {
		t.Fatalf("expected detail back esc key, got %#v", DetailBackKeys())
	}
	if got := DetailScrollKeys(); len(got) != 2 || got[0].String() != "pgdown" || got[1].String() != "end" {
		t.Fatalf("expected detail scroll sequence [pgdown end], got %#v", got)
	}
	model := InitializeModel(scenarioCounterModel{})
	model = ApplyKeySequence(model,
		Runes("x"),
		Runes("y"),
	)
	final, ok := model.(scenarioCounterModel)
	if !ok {
		t.Fatalf("expected scenarioCounterModel final type, got %T", model)
	}
	if final.count != 3 {
		t.Fatalf("expected init+typed rune count 3, got %d", final.count)
	}
}
