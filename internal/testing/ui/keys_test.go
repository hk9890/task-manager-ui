package ui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// keyFields is the part of a key message the helpers set, comparable with ==.
type keyFields struct {
	keyType tea.KeyType
	runes   string
	alt     bool
	paste   bool
}

func fieldsOf(key tea.KeyMsg) keyFields {
	return keyFields{key.Type, string(key.Runes), key.Alt, key.Paste}
}

func TestPressBuildsTheKeyItsNameNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		want       keyFields
		wantString string
	}{
		{"enter", keyFields{keyType: tea.KeyEnter}, "enter"},
		{"esc", keyFields{keyType: tea.KeyEsc}, "esc"},
		{"tab", keyFields{keyType: tea.KeyTab}, "tab"},
		{"shift+tab", keyFields{keyType: tea.KeyShiftTab}, "shift+tab"},
		{"up", keyFields{keyType: tea.KeyUp}, "up"},
		{"pgdown", keyFields{keyType: tea.KeyPgDown}, "pgdown"},
		{"home", keyFields{keyType: tea.KeyHome}, "home"},
		{"end", keyFields{keyType: tea.KeyEnd}, "end"},
		{"backspace", keyFields{keyType: tea.KeyBackspace}, "backspace"},
		{"delete", keyFields{keyType: tea.KeyDelete}, "delete"},
		{"ctrl+u", keyFields{keyType: tea.KeyCtrlU}, "ctrl+u"},
		{"ctrl+pgup", keyFields{keyType: tea.KeyCtrlPgUp}, "ctrl+pgup"},
		{"f1", keyFields{keyType: tea.KeyF1}, "f1"},
		{"alt+left", keyFields{keyType: tea.KeyLeft, alt: true}, "alt+left"},
		{"j", keyFields{keyType: tea.KeyRunes, runes: "j"}, "j"},
		{"alt+x", keyFields{keyType: tea.KeyRunes, runes: "x", alt: true}, "alt+x"},
		{"space", keyFields{keyType: tea.KeySpace, runes: " "}, " "},
	}
	for _, tc := range cases {
		got := Press(tc.name)
		if fields := fieldsOf(got); fields != tc.want {
			t.Errorf("Press(%q) = %+v, want %+v", tc.name, fields, tc.want)
		}
		if got.String() != tc.wantString {
			t.Errorf("Press(%q).String() = %q, want %q", tc.name, got.String(), tc.wantString)
		}
	}
}

func TestPressPanicsOnAnUnknownName(t *testing.T) {
	t.Parallel()

	defer func() {
		recovered := recover()
		message, ok := recovered.(string)
		if !ok || !strings.Contains(message, `unknown key name "ctrl+nope"`) {
			t.Fatalf("Press(%q) panic = %#v, want an unknown key name message", "ctrl+nope", recovered)
		}
	}()
	Press("ctrl+nope")
}

func TestEveryKeyNameIsTheNameOfTheKeyPressBuilds(t *testing.T) {
	t.Parallel()

	names := KeyNames()
	for _, name := range []string{"enter", "ctrl+t", "backspace", "f20"} {
		if !slices.Contains(names, name) {
			t.Errorf("KeyNames() lacks %q", name)
		}
	}
	for _, name := range names {
		if got := Press(name).String(); got != name {
			t.Errorf("Press(%q).String() = %q", name, got)
		}
	}
}

func TestTextKeyHelpersBuildRuneKeys(t *testing.T) {
	t.Parallel()

	cases := []struct {
		helper     string
		got        tea.KeyMsg
		want       keyFields
		wantString string
	}{
		{"Runes", Runes("hello"), keyFields{keyType: tea.KeyRunes, runes: "hello"}, "hello"},
		{"Runes", Runes("ü"), keyFields{keyType: tea.KeyRunes, runes: "ü"}, "ü"},
		{"Paste", Paste("a b"), keyFields{keyType: tea.KeyRunes, runes: "a b", paste: true}, "[a b]"},
	}
	for _, tc := range cases {
		if fields := fieldsOf(tc.got); fields != tc.want {
			t.Errorf("%s = %+v, want %+v", tc.helper, fields, tc.want)
		}
		if tc.got.String() != tc.wantString {
			t.Errorf("%s.String() = %q, want %q", tc.helper, tc.got.String(), tc.wantString)
		}
	}
}
