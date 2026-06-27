package details

// detailsTestWrapper adapts details.Model for use with the teatest harness.
// details.Model is not a full tea.Model (no Init/Update/View lifecycle);
// this wrapper holds terminal dimensions and wires key messages to HandleKey
// so that teatest can drive the model through real Bubble Tea message dispatch.

import (
	tea "github.com/charmbracelet/bubbletea"
)

// detailsTestWrapper wraps Model for teatest program use.
type detailsTestWrapper struct {
	m      *Model
	width  int
	height int
}

func newDetailsTestWrapper(m *Model, width, height int) detailsTestWrapper {
	return detailsTestWrapper{m: m, width: width, height: height}
}

func (w detailsTestWrapper) Init() tea.Cmd { return nil }

func (w detailsTestWrapper) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w.width = msg.Width
		w.height = msg.Height
	case tea.KeyMsg:
		w.m.HandleKey(msg, w.width, w.height)
	}
	return w, nil
}

func (w detailsTestWrapper) View() string {
	return w.m.View(w.width, w.height, false, 0)
}
