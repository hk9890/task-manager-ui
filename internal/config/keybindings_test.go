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

	if !resolved.Match(ShellContext, ShellActionQuit, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}) {
		t.Fatal("expected shell quit to match q")
	}
	if !resolved.Match(BoardContext, BoardActionMoveLeft, tea.KeyMsg{Type: tea.KeyLeft}) {
		t.Fatal("expected board move-left to match left arrow")
	}
	if resolved.Primary(ShellContext, ShellActionModeBoard) != "1" {
		t.Fatalf("expected primary board mode key to be 1, got %q", resolved.Primary(ShellContext, ShellActionModeBoard))
	}
	if resolved.Primary(ShellContext, ShellActionToggleSearch) != "ctrl+@" {
		t.Fatalf("expected search toggle key ctrl+@, got %q", resolved.Primary(ShellContext, ShellActionToggleSearch))
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

func TestResolveKeyBindingsRejectsInvalidKeys(t *testing.T) {
	t.Parallel()

	invalid := DefaultKeyBindings()
	invalid.Shell[ShellActionHelp] = []string{"bad key"}
	if _, err := ResolveKeyBindings(invalid); err == nil || !strings.Contains(err.Error(), "invalid key") {
		t.Fatalf("expected invalid key error, got %v", err)
	}
}
