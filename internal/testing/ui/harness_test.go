package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/hk9890/beads-workbench/internal/ui/modal"
)

type modalTeaModel struct {
	inner modal.Model
}

func (m modalTeaModel) Init() tea.Cmd {
	return m.inner.Init()
}

func (m modalTeaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.inner.Update(msg)
	return modalTeaModel{inner: next}, cmd
}

func (m modalTeaModel) View() string {
	return m.inner.View()
}

func TestHarnessSupportsTeaTestGoldenVerification(t *testing.T) {
	t.Parallel()

	m := modalTeaModel{inner: modal.New(modal.Config{Title: "Confirm", Message: "Ship it?"})}
	tm := NewTestModel(t, m)
	t.Cleanup(func() {
		_ = tm.Quit()
	})

	tm.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Ship it?")
	})
	if err := tm.Quit(); err != nil {
		t.Fatalf("failed to quit teatest model: %v", err)
	}

	tmModel := tm.FinalModel(t)
	modalModel, ok := tmModel.(modalTeaModel)
	if !ok {
		t.Fatalf("expected modalTeaModel final type, got %T", tmModel)
	}

	AssertModelViewMatchesGolden(t, modalModel, "modal_confirm.golden")
}
