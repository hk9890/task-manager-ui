package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// controllerStep is one message the recording controller saw, in arrival order.
type controllerStep string

// recordingController is a Controller that records every message reaching
// Update and answers a chosen message with a follow-up Cmd. It is a pointer
// receiver on purpose: Controller.Update returns only a Cmd, so a controller
// carries its own state rather than handing back a new value the way tea.Model
// does.
type recordingController struct {
	steps []controllerStep

	// initCmd is returned from Init, and replies maps a recorded step to the
	// Cmd emitted when that step arrives. Both model the async fan-out
	// ApplyControllerKeySequence exists to drain.
	initCmd tea.Cmd
	replies map[controllerStep]tea.Cmd
}

func (c *recordingController) Init() tea.Cmd { return c.initCmd }

func (c *recordingController) Update(msg tea.Msg) tea.Cmd {
	step := controllerStep(describeMsg(msg))
	c.steps = append(c.steps, step)

	return c.replies[step]
}

func (c *recordingController) View(int) string { return "" }

// describeMsg names a message compactly enough to assert an arrival order.
func describeMsg(msg tea.Msg) string {
	switch m := msg.(type) {
	case tea.KeyMsg:
		return "key:" + m.String()
	case string:
		return m
	default:
		return "unknown"
	}
}

func msgCmd(s string) tea.Cmd {
	return func() tea.Msg { return s }
}

func TestInitializeControllerDrainsTheInitCommand(t *testing.T) {
	t.Parallel()

	c := &recordingController{
		initCmd: msgCmd("init-msg"),
		replies: map[controllerStep]tea.Cmd{"init-msg": msgCmd("init-follow-up")},
	}

	got := InitializeController(c)

	if got != c {
		t.Fatalf("InitializeController returned a different controller: got %#v", got)
	}
	want := []controllerStep{"init-msg", "init-follow-up"}
	assertSteps(t, c.steps, want)
}

func TestInitializeControllerReturnsNilForANilController(t *testing.T) {
	t.Parallel()

	// The early return exists so a test that has not built a controller yet
	// gets nil back instead of a nil-pointer panic inside Init.
	if got := InitializeController(nil); got != nil {
		t.Fatalf("InitializeController(nil) = %#v, want nil", got)
	}
}

func TestApplyControllerKeySequenceDrainsEveryCommandBeforeTheNextKey(t *testing.T) {
	t.Parallel()

	// The "j" reply is a batch, so draining it must flatten the batch and feed
	// both nested messages back through Update before "k" is delivered. That
	// ordering is the whole contract the controller-async contract tests rest
	// on: no key arrives while a prior Cmd is still unresolved.
	c := &recordingController{
		replies: map[controllerStep]tea.Cmd{
			"key:j":     tea.Batch(msgCmd("j-a"), msgCmd("j-b")),
			"j-a":       msgCmd("j-a-nested"),
			"key:enter": msgCmd("enter-follow-up"),
		},
	}

	got := ApplyControllerKeySequence(c,
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")},
		tea.KeyMsg{Type: tea.KeyEnter},
	)

	if got != c {
		t.Fatalf("ApplyControllerKeySequence returned a different controller: got %#v", got)
	}
	want := []controllerStep{
		"key:j", "j-a", "j-b", "j-a-nested",
		"key:k",
		"key:enter", "enter-follow-up",
	}
	assertSteps(t, c.steps, want)
}

func TestApplyControllerKeySequenceWithNoKeysLeavesTheControllerUntouched(t *testing.T) {
	t.Parallel()

	c := &recordingController{}

	if got := ApplyControllerKeySequence(c); got != c {
		t.Fatalf("ApplyControllerKeySequence returned a different controller: got %#v", got)
	}
	if len(c.steps) != 0 {
		t.Fatalf("controller saw messages with no keys sent: %v", c.steps)
	}
}

func assertSteps(tb testing.TB, got, want []controllerStep) {
	tb.Helper()

	if len(got) != len(want) {
		tb.Fatalf("message order mismatch\ngot:  %v\nwant: %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			tb.Fatalf("message order mismatch at %d\ngot:  %v\nwant: %v", i, got, want)
		}
	}
}
