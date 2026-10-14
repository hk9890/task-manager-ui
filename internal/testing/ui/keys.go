package ui

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// A test builds every key message through Press, Runes and Paste, and only
// passes the result on. The key message type changes shape between
// Bubble Tea majors; when the construction lives here, this file is the only
// test code that changes with it.

const altPrefix = "alt+"

// namedKeys maps each key name Bubble Tea prints to its key type. It holds
// neither the runes type nor the space key, whose v1 name is " ": Press takes
// "space" for that key and reads a single rune as typed text.
var namedKeys = func() map[string]tea.KeyType {
	keys := map[string]tea.KeyType{}
	for keyType := tea.KeyF20; keyType <= tea.KeyBackspace; keyType++ {
		name := keyType.String()
		if name == "" || keyType == tea.KeyRunes || keyType == tea.KeySpace {
			continue
		}
		keys[name] = keyType
	}
	return keys
}()

// KeyNames returns, sorted, every name Press takes for a key that prints
// nothing: neither a rune nor "space".
func KeyNames() []string {
	return slices.Sorted(maps.Keys(namedKeys))
}

// Press returns the key message whose name is name, as Bubble Tea prints it
// and as the keybinding config writes it: "enter", "shift+tab", "ctrl+u",
// "f1", "alt+left", a single rune such as "j" or "alt+x", and "space" for the
// space key. It panics on any other name, because that is a bug in the test.
func Press(name string) tea.KeyMsg {
	key, alt := strings.CutPrefix(name, altPrefix)
	if utf8.RuneCountInString(key) == 1 {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key), Alt: alt}
	}
	if key == "space" {
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}, Alt: alt}
	}
	keyType, ok := namedKeys[key]
	if !ok {
		panic(fmt.Sprintf("testing/ui.Press: unknown key name %q", name))
	}
	return tea.KeyMsg{Type: keyType, Alt: alt}
}

// Runes returns the key message for typed text of any length.
func Runes(text string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)}
}

// Paste returns the key message for text pasted through bracketed paste.
func Paste(text string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true}
}
