package config

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestDefaultKeyBindingsResolveAndMatch(t *testing.T) {
	t.Parallel()

	resolved, err := ResolveKeyBindings(DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}

	if !resolved.Match(ShellContext, ShellActionQuit, tea.KeyMsg{Type: tea.KeyCtrlQ}) {
		t.Fatal("expected shell quit to match ctrl+q")
	}
	if !resolved.Match(BoardContext, BoardActionMoveLeft, tea.KeyMsg{Type: tea.KeyLeft}) {
		t.Fatal("expected board move-left to match left arrow")
	}
	if resolved.Primary(ShellContext, ShellActionModeBoard) != "1" {
		t.Fatalf("expected board mode key to be 1, got %q", resolved.Primary(ShellContext, ShellActionModeBoard))
	}
	if resolved.Primary(ShellContext, ShellActionToggleSearch) != "ctrl+@" {
		t.Fatalf("expected search toggle key ctrl+@, got %q", resolved.Primary(ShellContext, ShellActionToggleSearch))
	}
	if resolved.Primary(ShellContext, ShellActionModeCycleNext) != "ctrl+pgdown" {
		t.Fatalf("expected mode cycle next key to be ctrl+pgdown, got %q", resolved.Primary(ShellContext, ShellActionModeCycleNext))
	}
}

func TestMergeKeyBindingsOverridesPerAction(t *testing.T) {
	t.Parallel()

	merged := MergeKeyBindings(DefaultKeyBindings(), &KeyBindingOverride{
		Shell: map[string][]string{ShellActionQuit: {"ctrl+q"}},
		Board: map[string][]string{BoardActionMoveLeft: {"a"}},
	})

	if got := strings.Join(merged.Shell[ShellActionQuit], ","); got != "ctrl+q" {
		t.Fatalf("expected shell quit override, got %q", got)
	}
	if got := strings.Join(merged.Board[BoardActionMoveLeft], ","); got != "a" {
		t.Fatalf("expected board move-left override, got %q", got)
	}
	if got := strings.Join(merged.Board[BoardActionMoveRight], ","); got != "l,right,tab" {
		t.Fatalf("expected unspecified bindings to remain, got %q", got)
	}
}

func TestResolveKeyBindingsRejectsConflictsAndUnknownActions(t *testing.T) {
	t.Parallel()

	conflicting := DefaultKeyBindings()
	conflicting.Board[BoardActionMoveLeft] = []string{"h"}
	conflicting.Board[BoardActionMoveRight] = []string{"h"}
	if _, err := ResolveKeyBindings(conflicting); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected conflict error, got %v", err)
	}

	unknown := DefaultKeyBindings()
	unknown.Shell["mystery"] = []string{"z"}
	if _, err := ResolveKeyBindings(unknown); err == nil || !strings.Contains(err.Error(), "unknown keybinding action") {
		t.Fatalf("expected unknown action error, got %v", err)
	}
}

func TestMergeKeyBindingsDoesNotMutateBase(t *testing.T) {
	t.Parallel()

	// Capture the original default before any merge.
	original := DefaultKeyBindings()
	originalQuitKeys := append([]string(nil), original.Shell[ShellActionQuit]...)

	// Apply an override that changes shell quit.
	_ = MergeKeyBindings(DefaultKeyBindings(), &KeyBindingOverride{
		Shell: map[string][]string{ShellActionQuit: {"ctrl+c"}},
	})

	// The original base must be unchanged.
	after := DefaultKeyBindings()
	afterQuitKeys := after.Shell[ShellActionQuit]
	if len(afterQuitKeys) != len(originalQuitKeys) {
		t.Fatalf("DefaultKeyBindings shell quit mutated: before=%v after=%v", originalQuitKeys, afterQuitKeys)
	}
	for i := range originalQuitKeys {
		if originalQuitKeys[i] != afterQuitKeys[i] {
			t.Fatalf("DefaultKeyBindings shell quit mutated at index %d: before=%v after=%v", i, originalQuitKeys, afterQuitKeys)
		}
	}
}

func TestResolveKeyBindingsRejectsInvalidKeys(t *testing.T) {
	t.Parallel()

	invalid := DefaultKeyBindings()
	invalid.Shell[ShellActionHelp] = []string{"bad key"}
	if _, err := ResolveKeyBindings(invalid); err == nil || !strings.Contains(err.Error(), "invalid key") {
		t.Fatalf("expected invalid key error, got %v", err)
	}
}

func resolvedDefault(t *testing.T) ResolvedKeyBindings {
	t.Helper()
	resolved, err := ResolveKeyBindings(DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings returned error: %v", err)
	}
	return resolved
}

func TestDisplayKeyName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"ctrl+@", "ctrl+space"},
		{"ctrl+space", "ctrl+space"},
		{"q", "q"},
		{"ctrl+q", "ctrl+q"},
		{"left", "left"},
		{"f13", "f13"},
		{"", ""},
		{"Space", "space"},
		{" ", "space"},
	}

	for _, tc := range tests {
		got := DisplayKeyName(tc.input)
		if got != tc.want {
			t.Errorf("DisplayKeyName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestResolvedKeyBindingsDisplayPrimary(t *testing.T) {
	t.Parallel()

	r := resolvedDefault(t)

	// The toggle-search action uses ctrl+@ canonical; DisplayPrimary must return "ctrl+space".
	got := r.DisplayPrimary(ShellContext, ShellActionToggleSearch)
	if got != "ctrl+space" {
		t.Errorf("DisplayPrimary(shell, toggle_search) = %q, want %q", got, "ctrl+space")
	}

	// A regular single-char binding: shell quit is ctrl+q.
	got = r.DisplayPrimary(ShellContext, ShellActionQuit)
	if got != "ctrl+q" {
		t.Errorf("DisplayPrimary(shell, quit) = %q, want %q", got, "ctrl+q")
	}
}

func TestResolvedKeyBindingsDisplayPrimaryMissingContextOrAction(t *testing.T) {
	t.Parallel()

	r := resolvedDefault(t)

	if got := r.DisplayPrimary("nosuchcontext", ShellActionQuit); got != "" {
		t.Errorf("DisplayPrimary(missing context) = %q, want empty", got)
	}
	if got := r.DisplayPrimary(ShellContext, "nosuchaction"); got != "" {
		t.Errorf("DisplayPrimary(missing action) = %q, want empty", got)
	}
}

func TestResolvedKeyBindingsDisplayLabel(t *testing.T) {
	t.Parallel()

	r := resolvedDefault(t)

	// board move-right has keys "l", "right", "tab" → label is "l/right/tab".
	got := r.DisplayLabel(BoardContext, BoardActionMoveRight)
	if got != "l/right/tab" {
		t.Errorf("DisplayLabel(board, move_right) = %q, want %q", got, "l/right/tab")
	}

	// Missing context / action must return "".
	if got := r.DisplayLabel("nosuch", BoardActionMoveRight); got != "" {
		t.Errorf("DisplayLabel(missing context) = %q, want empty", got)
	}
	if got := r.DisplayLabel(BoardContext, "nosuchaction"); got != "" {
		t.Errorf("DisplayLabel(missing action) = %q, want empty", got)
	}
}

func TestResolvedKeyBindingsIsZero(t *testing.T) {
	t.Parallel()

	// Zero value must report IsZero == true.
	var zero ResolvedKeyBindings
	if !zero.IsZero() {
		t.Fatal("expected zero-value ResolvedKeyBindings to report IsZero() == true")
	}

	// Resolved from defaults must report IsZero == false.
	r := resolvedDefault(t)
	if r.IsZero() {
		t.Fatal("expected populated ResolvedKeyBindings to report IsZero() == false")
	}
}
