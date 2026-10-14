package ui

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// A test builds every key message through Press, Runes and Paste, and only
// passes the result on. The key message type changes shape between
// Bubble Tea majors; when the construction lives here, this file is the only
// test code that changes with it.

// modifiers maps each prefix of a key name to its modifier.
var modifiers = map[string]tea.KeyMod{
	"ctrl":  tea.ModCtrl,
	"alt":   tea.ModAlt,
	"shift": tea.ModShift,
}

// namedKeys maps the name of each key that prints nothing to its code.
var namedKeys = func() map[string]rune {
	keys := map[string]rune{
		"enter":     tea.KeyEnter,
		"tab":       tea.KeyTab,
		"backspace": tea.KeyBackspace,
		"esc":       tea.KeyEscape,
		"up":        tea.KeyUp,
		"down":      tea.KeyDown,
		"left":      tea.KeyLeft,
		"right":     tea.KeyRight,
		"home":      tea.KeyHome,
		"end":       tea.KeyEnd,
		"pgup":      tea.KeyPgUp,
		"pgdown":    tea.KeyPgDown,
		"delete":    tea.KeyDelete,
		"insert":    tea.KeyInsert,
	}
	for n := range 20 {
		keys[fmt.Sprintf("f%d", n+1)] = tea.KeyF1 + rune(n)
	}
	return keys
}()

// KeyNames returns, sorted, every name of a key that prints nothing: each
// named key alone and under ctrl, ctrl with each letter and with space, and
// shift+tab.
func KeyNames() []string {
	names := []string{"shift+tab", "ctrl+space"}
	for name := range namedKeys {
		names = append(names, name, "ctrl+"+name)
	}
	for letter := 'a'; letter <= 'z'; letter++ {
		names = append(names, "ctrl+"+string(letter))
	}
	slices.Sort(names)
	return names
}

// Press returns the key message whose name is name, as Bubble Tea prints it
// and as the keybinding config writes it: "enter", "shift+tab", "ctrl+u",
// "f1", "alt+left", a single rune such as "j", "J" or "alt+x", and "space" for
// the space key. It panics on any other name, because that is a bug in the
// test.
func Press(name string) tea.KeyPressMsg {
	var mod tea.KeyMod
	key := name
	for {
		prefix, rest, found := strings.Cut(key, "+")
		modifier, known := modifiers[prefix]
		if !found || !known || rest == "" {
			break
		}
		mod |= modifier
		key = rest
	}

	var msg tea.KeyPressMsg
	switch code, isNamed := namedKeys[key]; {
	case key == "space":
		msg = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case utf8.RuneCountInString(key) == 1:
		msg = runeKey([]rune(key)[0])
	case isNamed:
		msg = tea.KeyPressMsg{Code: code}
	default:
		panic(fmt.Sprintf("testing/ui.Press: unknown key name %q", name))
	}

	msg.Mod |= mod
	// A terminal reports the text of a key only when neither ctrl nor alt
	// is held.
	if mod&(tea.ModCtrl|tea.ModAlt) != 0 {
		msg.Text = ""
	}
	return msg
}

// runeKey is the press of the key that types r. An upper-case letter is its
// lower-case key with shift, as a terminal reports it.
func runeKey(r rune) tea.KeyPressMsg {
	if unicode.IsUpper(r) {
		return tea.KeyPressMsg{Code: unicode.ToLower(r), ShiftedCode: r, Mod: tea.ModShift, Text: string(r)}
	}
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

// Runes returns the key message for typed text: one rune is its key, and
// several are one key that types them all, as an input method sends them.
func Runes(text string) tea.KeyPressMsg {
	if utf8.RuneCountInString(text) == 1 {
		r, _ := utf8.DecodeRuneInString(text)
		return runeKey(r)
	}
	return tea.KeyPressMsg{Code: tea.KeyExtended, Text: text}
}

// Paste returns the message for text pasted through bracketed paste.
func Paste(text string) tea.PasteMsg {
	return tea.PasteMsg{Content: text}
}
