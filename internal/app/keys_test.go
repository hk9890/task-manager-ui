package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// namedKeys are the non-rune keys the tests press, by the name a binding
// gives them.
var namedKeys = map[string]tea.KeyType{
	"enter":     tea.KeyEnter,
	"esc":       tea.KeyEsc,
	"tab":       tea.KeyTab,
	"shift+tab": tea.KeyShiftTab,
	"up":        tea.KeyUp,
	"down":      tea.KeyDown,
	"left":      tea.KeyLeft,
	"right":     tea.KeyRight,
	"home":      tea.KeyHome,
	"end":       tea.KeyEnd,
	"pgup":      tea.KeyPgUp,
	"pgdown":    tea.KeyPgDown,
	"delete":    tea.KeyDelete,
	"ctrl+c":    tea.KeyCtrlC,
	"ctrl+t":    tea.KeyCtrlT,
}

// testKey is the key press a binding named name matches: a named key, alt+
// and a rune, or the runes themselves.
func testKey(name string) tea.KeyMsg {
	if keyType, ok := namedKeys[name]; ok {
		return tea.KeyMsg{Type: keyType}
	}
	if rest, ok := strings.CutPrefix(name, "alt+"); ok {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(rest), Alt: true}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
}
