package configscreen

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

var (
	keyUp    = tea.KeyMsg{Type: tea.KeyUp}
	keyDown  = tea.KeyMsg{Type: tea.KeyDown}
	keyLeft  = tea.KeyMsg{Type: tea.KeyLeft}
	keyRight = tea.KeyMsg{Type: tea.KeyRight}
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
)

func newModel(t *testing.T, theme, glyphs string) *Model {
	t.Helper()

	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings: %v", err)
	}
	m := NewModel(keys)
	m.SetSize(80, 12)
	m.Open("/tmp/config.yaml", theme, glyphs)
	return m
}

// press sends one key the screen must consume and returns the change it asks
// the shell for, or nil when it asks for none.
func press(t *testing.T, m *Model, key tea.KeyMsg) *ChangeMsg {
	t.Helper()

	consumed, cmd := m.HandleKey(key)
	if !consumed {
		t.Fatalf("%q was not consumed", key.String())
	}
	if cmd == nil {
		return nil
	}
	change, ok := cmd().(ChangeMsg)
	if !ok {
		t.Fatalf("%q produced %T, want a ChangeMsg", key.String(), cmd())
	}
	return &change
}

func TestUpAndDownMoveTheCursorAndStopAtBothEnds(t *testing.T) {
	t.Parallel()

	m := newModel(t, "catppuccin-mocha", "unicode")
	steps := []struct {
		key  tea.KeyMsg
		want int
	}{
		{keyUp, rowTheme}, {keyDown, rowGlyphs}, {keyDown, rowGlyphs}, {keyUp, rowTheme}, {keyUp, rowTheme},
	}
	for idx, step := range steps {
		if change := press(t, m, step.key); change != nil {
			t.Fatalf("step %d: %q asked for a change: %+v", idx, step.key.String(), *change)
		}
		if m.selectedRow != step.want {
			t.Fatalf("step %d: %q left the cursor on row %d, want %d", idx, step.key.String(), m.selectedRow, step.want)
		}
	}
}

// Left, Right and Enter ask for the neighbour of the selected row's value in
// the sorted list, and the other value as it is.
func TestStepKeysAskForTheNeighbourAndWrap(t *testing.T) {
	t.Parallel()

	themes, sets := styles.Themes(), styles.GlyphSets()
	firstTheme, lastTheme := themes[0], themes[len(themes)-1]
	firstSet, lastSet := sets[0], sets[len(sets)-1]

	cases := []struct {
		name          string
		theme, glyphs string
		row           int
		key           tea.KeyMsg
		want          ChangeMsg
	}{
		{"right steps the theme", firstTheme, firstSet, rowTheme, keyRight, ChangeMsg{Theme: themes[1], Glyphs: firstSet}},
		{"enter steps the theme as right does", firstTheme, firstSet, rowTheme, keyEnter, ChangeMsg{Theme: themes[1], Glyphs: firstSet}},
		{"left steps the theme back", themes[1], firstSet, rowTheme, keyLeft, ChangeMsg{Theme: firstTheme, Glyphs: firstSet}},
		{"right wraps after the last theme", lastTheme, firstSet, rowTheme, keyRight, ChangeMsg{Theme: firstTheme, Glyphs: firstSet}},
		{"left wraps before the first theme", firstTheme, firstSet, rowTheme, keyLeft, ChangeMsg{Theme: lastTheme, Glyphs: firstSet}},
		{"right steps the glyph set", firstTheme, firstSet, rowGlyphs, keyRight, ChangeMsg{Theme: firstTheme, Glyphs: sets[1]}},
		{"right wraps after the last glyph set", firstTheme, lastSet, rowGlyphs, keyEnter, ChangeMsg{Theme: firstTheme, Glyphs: firstSet}},
		{"left wraps before the first glyph set", firstTheme, firstSet, rowGlyphs, keyLeft, ChangeMsg{Theme: firstTheme, Glyphs: lastSet}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := newModel(t, tc.theme, tc.glyphs)
			m.selectedRow = tc.row

			change := press(t, m, tc.key)
			if change == nil || *change != tc.want {
				t.Fatalf("asked for %+v, want %+v", change, tc.want)
			}
			// The shell makes the change; until it says so the screen shows
			// what is in use.
			if m.theme != tc.theme || m.glyphs != tc.glyphs {
				t.Errorf("the screen changed its own values to %q, %q", m.theme, m.glyphs)
			}
		})
	}
}

func TestSetValuesIsWhatTheNextStepStartsFrom(t *testing.T) {
	t.Parallel()

	themes := styles.Themes()
	m := newModel(t, themes[0], "unicode")
	m.SetValues(themes[1], "unicode")

	if change := press(t, m, keyRight); change == nil || change.Theme != themes[2] {
		t.Fatalf("asked for %+v, want the theme %q", change, themes[2])
	}
	if view := m.View(""); !strings.Contains(view, themes[1]) {
		t.Errorf("the screen does not show the theme in use, %q:\n%s", themes[1], view)
	}
}

func TestOpenPutsTheCursorOnTheFirstRow(t *testing.T) {
	t.Parallel()

	m := newModel(t, "catppuccin-mocha", "unicode")
	press(t, m, keyDown)
	m.Open("/tmp/other.yaml", "catppuccin-latte", "ascii")

	if m.selectedRow != rowTheme {
		t.Errorf("the cursor is on row %d after Open, want the first", m.selectedRow)
	}
	view := m.View("")
	testui.AssertContainsAll(t, view, "/tmp/other.yaml", "catppuccin-latte", "ascii")
}

// A key the screen does not claim falls through to the shell: that is how
// Escape, quit and help keep working, and how an alt+ chord stays a shell key.
func TestUnclaimedKeysFallThrough(t *testing.T) {
	t.Parallel()

	unclaimed := map[string]tea.KeyMsg{
		"esc":       {Type: tea.KeyEsc},
		"ctrl+c":    {Type: tea.KeyCtrlC},
		"alt+h":     {Type: tea.KeyRunes, Runes: []rune("h"), Alt: true},
		"tab":       {Type: tea.KeyTab},
		"a letter":  {Type: tea.KeyRunes, Runes: []rune("x")},
		"alt+left":  {Type: tea.KeyLeft, Alt: true},
		"alt+right": {Type: tea.KeyRight, Alt: true},
		"alt+enter": {Type: tea.KeyEnter, Alt: true},
		"pgdown":    {Type: tea.KeyPgDown},
	}
	for name, key := range unclaimed {
		m := newModel(t, "catppuccin-mocha", "unicode")
		consumed, cmd := m.HandleKey(key)
		if consumed || cmd != nil {
			t.Errorf("%s: consumed %v, command %v; want the key left to the shell", name, consumed, cmd != nil)
		}
		if m.selectedRow != rowTheme {
			t.Errorf("%s moved the cursor", name)
		}
	}
}

// Row movement reads the board bindings, so a rebound move key moves here too.
func TestMoveKeysFollowTheBoardBindings(t *testing.T) {
	t.Parallel()

	bindings := config.DefaultKeyBindings()
	bindings.Board[config.BoardActionMoveDown] = []string{"ctrl+n"}
	keys, err := config.ResolveKeyBindings(bindings)
	if err != nil {
		t.Fatalf("ResolveKeyBindings: %v", err)
	}
	m := NewModel(keys)
	m.Open("", "catppuccin-mocha", "unicode")

	if consumed, _ := m.HandleKey(keyDown); consumed {
		t.Error("down moved the cursor after move_down was rebound")
	}
	if consumed, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlN}); !consumed || m.selectedRow != rowGlyphs {
		t.Errorf("ctrl+n: consumed %v, cursor on row %d; want the second row", consumed, m.selectedRow)
	}
}
