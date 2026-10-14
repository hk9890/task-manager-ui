package ui

import (
	"slices"
	"strings"
	"testing"
)

func TestPressBuildsTheKeyItsNameNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		wantString    string
		wantKeystroke string
	}{
		{"enter", "enter", "enter"},
		{"esc", "esc", "esc"},
		{"tab", "tab", "tab"},
		{"shift+tab", "shift+tab", "shift+tab"},
		{"up", "up", "up"},
		{"pgdown", "pgdown", "pgdown"},
		{"home", "home", "home"},
		{"end", "end", "end"},
		{"backspace", "backspace", "backspace"},
		{"delete", "delete", "delete"},
		{"ctrl+u", "ctrl+u", "ctrl+u"},
		{"ctrl+pgup", "ctrl+pgup", "ctrl+pgup"},
		{"ctrl+space", "ctrl+space", "ctrl+space"},
		{"f1", "f1", "f1"},
		{"alt+left", "alt+left", "alt+left"},
		{"j", "j", "j"},
		{"J", "J", "shift+j"},
		{"shift+j", "J", "shift+j"},
		{"numlock+left", "left", "left"},
		{"kpenter", "enter", "enter"},
		{"kpleft", "left", "left"},
		{"alt+x", "alt+x", "alt+x"},
		{"space", "space", "space"},
		{"alt+space", "alt+space", "alt+space"},
	}
	for _, tc := range cases {
		got := Press(tc.name)
		if got.String() != tc.wantString {
			t.Errorf("Press(%q).String() = %q, want %q", tc.name, got.String(), tc.wantString)
		}
		if got.Keystroke() != tc.wantKeystroke {
			t.Errorf("Press(%q).Keystroke() = %q, want %q", tc.name, got.Keystroke(), tc.wantKeystroke)
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

func TestTextKeyHelpersBuildTextKeys(t *testing.T) {
	t.Parallel()

	cases := []struct {
		text          string
		wantKeystroke string
	}{
		{"hello", "hello"},
		{"ü", "ü"},
		{"Q", "shift+q"},
	}
	for _, tc := range cases {
		got := Runes(tc.text)
		if got.String() != tc.text {
			t.Errorf("Runes(%q).String() = %q, want the text", tc.text, got.String())
		}
		if got.Keystroke() != tc.wantKeystroke {
			t.Errorf("Runes(%q).Keystroke() = %q, want %q", tc.text, got.Keystroke(), tc.wantKeystroke)
		}
	}

	if got := Paste("a b").Content; got != "a b" {
		t.Errorf("Paste content = %q, want %q", got, "a b")
	}
}
