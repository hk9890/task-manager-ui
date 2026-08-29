package ui

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// firstSizeModel renders the first WindowSizeMsg it receives and ignores every
// later one. Recording only the first is the point: the size teatest sends at
// startup is the harness default under test, and a test that sends its own size
// afterwards — as the golden test does — would otherwise overwrite the evidence.
type firstSizeModel struct {
	mu    *sync.Mutex
	first *tea.WindowSizeMsg
}

func newFirstSizeModel() firstSizeModel {
	return firstSizeModel{mu: &sync.Mutex{}, first: &tea.WindowSizeMsg{}}
}

func (m firstSizeModel) Init() tea.Cmd { return nil }

func (m firstSizeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.mu.Lock()
		if m.first.Width == 0 && m.first.Height == 0 {
			*m.first = size
		}
		m.mu.Unlock()
	}

	return m, nil
}

func (m firstSizeModel) View() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return fmt.Sprintf("initial-size w=%d h=%d", m.first.Width, m.first.Height)
}

func assertInitialTermSize(t *testing.T, tm *teatest.TestModel, wantWidth, wantHeight int) {
	t.Helper()

	want := fmt.Sprintf("initial-size w=%d h=%d", wantWidth, wantHeight)
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), want)
	})
	if err := tm.Quit(); err != nil {
		t.Fatalf("failed to quit teatest model: %v", err)
	}
}

// The harness hands every teatest-based UI test its terminal geometry. A
// transposed width and height there would pin the wrong layout branch in every
// golden captured through it, while each individual test still passed.
func TestNewTestModelStartsAtTheDefaultTerminalSize(t *testing.T) {
	t.Parallel()

	tm := NewTestModel(t, newFirstSizeModel())
	t.Cleanup(func() { _ = tm.Quit() })

	assertInitialTermSize(t, tm, defaultWidth, defaultHeight)
}

func TestNewTestModelWithSizeStartsAtTheRequestedSize(t *testing.T) {
	t.Parallel()

	tm := NewTestModelWithSize(t, newFirstSizeModel(), 120, 40)
	t.Cleanup(func() { _ = tm.Quit() })

	assertInitialTermSize(t, tm, 120, 40)
}

func TestNewTestModelWithSizeFallsBackToTheDefaultForNonPositiveDimensions(t *testing.T) {
	t.Parallel()

	tm := NewTestModelWithSize(t, newFirstSizeModel(), 0, -1)
	t.Cleanup(func() { _ = tm.Quit() })

	assertInitialTermSize(t, tm, defaultWidth, defaultHeight)
}
