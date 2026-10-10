package app

// The configuration screen: what opens it, what it renders instead of, what
// leaving it restores, and what a change does.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// configShell is filterShell with a config file of its own to write to. The
// file does not exist yet, as on a first start.
func configShell(t *testing.T) (Model, string) {
	t.Helper()

	m, _ := filterShell(t)
	path := filepath.Join(t.TempDir(), "taskmgr-ui", "config.yaml")
	m.services.ConfigPath = path
	return m, path
}

func openConfigScreen(t *testing.T, m Model) Model {
	t.Helper()

	m = press(t, m, "alt+c")
	if m.active != mode.Config {
		t.Fatalf("active mode after the config key: got %q, want %q", m.active, mode.Config)
	}
	return m
}

// restoreStyles is for a test that changes the theme or the glyph set: both
// are package variables of internal/ui/styles. Such a test is not parallel,
// and puts back what that package starts on.
func restoreStyles(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		if err := styles.Apply("catppuccin-mocha", "unicode"); err != nil {
			t.Fatalf("restore the initial styles: %v", err)
		}
	})
}

func TestConfigScreenOpensOnItsKeyAndShowsWhatIsInUse(t *testing.T) {
	m, _ := configShell(t)
	m = openConfigScreen(t, m)

	if m.configReturn != mode.Board {
		t.Errorf("the screen returns to %q, want the board it was opened from", m.configReturn)
	}
	// The end of the path only: a temp directory can be longer than the line,
	// and the screen then cuts the path from its front.
	testui.AssertContainsAll(t, plainShell(m),
		"Configuration", "taskmgr-ui/config.yaml as it changes",
		"Appearance", "▌ theme     ‹ catppuccin-mocha ›", "  glyphs    ‹ unicode ›",
		"down/up move · left/right change · esc back · ctrl+c quit",
	)
}

// The screen renders instead of the shell, so the three header lines and the
// shell footer are absent while it is up (docs/DESIGN-GUIDE.md).
func TestConfigScreenReplacesTheShellChrome(t *testing.T) {
	m, _ := configShell(t)

	chrome := strings.Split(testui.AnsiEscapePattern.ReplaceAllString(m.renderHeader(), ""), "\n")
	footer := testui.AnsiEscapePattern.ReplaceAllString(m.renderFooter(), "")
	before := plainShell(m)
	for _, line := range append(chrome, footer) {
		if strings.TrimSpace(line) == "" || !strings.Contains(before, line) {
			t.Fatalf("fixture: the shell does not draw the chrome line %q:\n%s", line, before)
		}
	}

	m = openConfigScreen(t, m)
	view := plainShell(m)
	for _, gone := range append(chrome, footer, " Board ", " Docs ", "config alt+c", "Fix login prompt") {
		if strings.Contains(view, gone) {
			t.Errorf("%q is still rendered under the configuration screen:\n%s", gone, view)
		}
	}
	if lines := strings.Split(view, "\n"); len(lines) != m.height {
		t.Errorf("the screen draws %d lines on a terminal of %d", len(lines), m.height)
	}
}

// Escape puts the operator back on the surface the screen was opened from,
// drawn as it was: the selection, the query and the detail are untouched.
func TestConfigScreenEscapeReturnsToTheSurfaceItWasOpenedFrom(t *testing.T) {
	cases := []struct {
		name string
		from mode.ID
		// keys take filterShell from the Board to the surface; typed is the
		// query typed there.
		keys  []string
		typed string
	}{
		{name: "the board with a moved selection and a filter", from: mode.Board, keys: []string{"right"}, typed: "login"},
		{name: "the docs tab", from: mode.Docs, keys: []string{"tab", "down"}},
		{name: "the store search with a query", from: mode.Search, keys: []string{"alt+f"}, typed: "login"},
		{name: "detail", from: mode.Detail, keys: []string{"down", "enter"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := configShell(t)
			m = press(t, m, tc.keys...)
			if tc.typed != "" {
				m = typeInto(t, m, tc.typed)
			}
			if m.active != tc.from {
				t.Fatalf("fixture: on %q, want %q", m.active, tc.from)
			}
			before, selected, searchFrom := plainShell(m), m.currentSelection(), m.searchFrom
			if selected == nil {
				t.Fatal("fixture: the surface has no selection")
			}

			m = openConfigScreen(t, m)
			if m.configReturn != tc.from {
				t.Fatalf("the screen returns to %q, want %q", m.configReturn, tc.from)
			}
			m = press(t, m, "down", "up", "esc")

			if m.active != tc.from || m.searchFrom != searchFrom {
				t.Fatalf("Escape landed on %q (search from %q), want %q (search from %q)", m.active, m.searchFrom, tc.from, searchFrom)
			}
			if got := m.currentSelection(); got == nil || got.Issue.ID != selected.Issue.ID {
				t.Errorf("the selection is %v, want %q as before the screen", got, selected.Issue.ID)
			}
			if got := plainShell(m); got != before {
				t.Errorf("the surface is drawn differently after the round trip:\n%s\n--- before ---\n%s", got, before)
			}
		})
	}
}

// The screen is not a tab: it is not in the strip, and the tab keys do not
// leave it for one.
func TestConfigScreenIsNotInTheTabCycle(t *testing.T) {
	if mode.IsBrowse(mode.Config) {
		t.Fatalf("the browse tabs are %v, want no configuration screen", mode.BrowseModes)
	}

	m, _ := configShell(t)
	for i := 0; i < len(mode.BrowseModes)+2; i++ {
		if m = press(t, m, "tab"); m.active == mode.Config {
			t.Fatal("tab cycling reached the configuration screen; it is not a tab")
		}
	}

	m = openConfigScreen(t, m)
	if m = press(t, m, "tab", "shift+tab"); m.active != mode.Config {
		t.Errorf("a tab key left the configuration screen for %q", m.active)
	}
}

// Every shell action but Escape, quit and help is inert on the screen: the
// issue actions would act on a row that is not on screen, and the others
// would put a surface over one that is itself above the shell.
func TestShellActionsAreInertOnTheConfigScreen(t *testing.T) {
	for _, key := range []string{
		"alt+e", "alt+n", "alt+u", "delete", "alt+a",
		"alt+s", "alt+f", "alt+r", "alt+c", "alt+v", "alt+p", "alt+l",
	} {
		t.Run(key, func(t *testing.T) {
			m, _ := configShell(t)
			m = openConfigScreen(t, m)
			before := plainShell(m)

			next, cmd := m.Update(testKey(key))
			if cmd != nil {
				t.Errorf("%q returned a command from the configuration screen", key)
			}
			m = next.(Model)

			if m.showActionModal || m.pendingDialog.active {
				t.Errorf("%q armed a dialog (modal %v, pending %v)", key, m.showActionModal, m.pendingDialog.active)
			}
			if m.active != mode.Config || m.configReturn != mode.Board || m.searchFrom != "" {
				t.Errorf("%q moved the operator: on %q returning to %q, search from %q", key, m.active, m.configReturn, m.searchFrom)
			}
			if got := plainShell(m); got != before {
				t.Errorf("%q changed the screen:\n%s", key, got)
			}
		})
	}
}

func TestQuitAndHelpWorkOnTheConfigScreen(t *testing.T) {
	m, _ := configShell(t)
	m = openConfigScreen(t, m)

	_, cmd := m.Update(testKey("ctrl+c"))
	if !isQuitCmd(cmd) {
		t.Error("the quit key does not quit from the configuration screen")
	}

	m = press(t, m, "alt+h")
	if !m.showHelp || !strings.Contains(plainShell(m), "Keyboard Help") {
		t.Fatalf("the help overlay is not drawn over the configuration screen (showHelp %v):\n%s", m.showHelp, plainShell(m))
	}
	if help := shellKeyHelp(m.keys); !strings.Contains(help, "alt+c = open the configuration screen") {
		t.Errorf("the help does not name the configuration key:\n%s", help)
	}
	// Help holds the keyboard: a step key under it changes nothing.
	if m = press(t, m, "right", "alt+h"); m.showHelp || m.active != mode.Config || m.services.Config.UI.Theme != "catppuccin-mocha" {
		t.Errorf("after help closed: help %v, on %q, theme %q", m.showHelp, m.active, m.services.Config.UI.Theme)
	}
}

// The screen does not open over the store picker, as the store search does
// not.
func TestConfigScreenDoesNotOpenFromThePicker(t *testing.T) {
	m := mustNewModel(t, pickerServices(t, &fakes.FakeStoreCatalog{Entries: registryEntries()}, ""))
	m = applyMessages(t, m, runBatch(m.Init()))
	m = openPicker(t, m)

	if m = press(t, m, "alt+c"); m.active != mode.StorePicker {
		t.Errorf("the config key left the picker for %q", m.active)
	}
}

// The screen ignores the mouse, and the header is not drawn under it: a click
// where a button or a tab was runs nothing and lights nothing.
func TestTheMouseIsDeadOnTheConfigScreen(t *testing.T) {
	base := newMouseShell(t)
	cells := map[string][2]int{}
	for label, action := range map[string]string{
		"stores": config.ShellActionStorePicker, "config": config.ShellActionOpenConfig,
		"help": config.ShellActionHelp, "quit": config.ShellActionQuit,
	} {
		x, _ := barButton(t, base, label, action)
		cells[label] = [2]int{x, headerMenuRow}
	}
	docsX, _ := testui.FindCell(t, tabLine(base), " Docs ")
	cells["the docs tab"] = [2]int{docsX + 1, headerTabsRow}
	cells["a row"] = [2]int{4, 5}

	m := pressKey(t, base, "alt+c")
	if m.active != mode.Config {
		t.Fatalf("fixture did not open the configuration screen, on %q", m.active)
	}
	before := m.View()
	for name, cell := range cells {
		for _, event := range []tea.MouseMsg{pointerMove(cell[0], cell[1]), leftClick(cell[0], cell[1]), leftClick(cell[0], cell[1])} {
			next, cmd := m.Update(event)
			m = next.(Model)
			if cmd != nil {
				t.Errorf("a mouse event on %s returned a command under the configuration screen", name)
			}
		}
		if m.active != mode.Config || m.showHelp || litButton(m) != "" || m.hoverTab != "" {
			t.Errorf("a click on %s ran under the configuration screen (surface %q, help %v, hover %q/%q)",
				name, m.active, m.showHelp, m.hoverTab, litButton(m))
		}
		if m.View() != before {
			t.Errorf("a click on %s changed the configuration screen", name)
		}
	}

	// The wheel moves no cursor either.
	next, _ := m.Update(tea.MouseMsg{X: 4, Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if next.(Model).View() != before {
		t.Error("the wheel changed the configuration screen")
	}
}

// A browse tab's background load landing under the screen must not retarget
// the Detail surface, for the reason it must not under the picker.
func TestSelectionLandingUnderTheConfigScreenDoesNotLoadDetail(t *testing.T) {
	m, _ := configShell(t)
	m = press(t, m, "enter")
	if m.active != mode.Detail || m.detail.Detail.Summary.ID != "tm-1" {
		t.Fatalf("fixture: on %q showing %q, want detail on tm-1", m.active, m.detail.Detail.Summary.ID)
	}
	m = openConfigScreen(t, m)

	next, cmd := m.Update(mode.SelectionChangedMsg{
		Mode:      mode.Docs,
		Selection: &mode.Selection{Issue: domain.IssueSummary{ID: "tm-9", Title: "Session notes", Status: "open", Type: "doc"}},
	})
	m = applyMessages(t, next.(Model), runBatch(cmd))
	next, cmd = m.Update(mode.SelectionChangedMsg{
		Mode:      mode.Board,
		Selection: &mode.Selection{Issue: domain.IssueSummary{ID: "tm-2", Title: "Triage inbox", Status: "open", Type: "task"}},
	})
	if cmd != nil {
		t.Error("a selection landing under the configuration screen returned a command")
	}
	m = next.(Model)

	if m.detail.IsLoading() || strings.TrimSpace(m.detail.TargetID()) != "tm-1" {
		t.Errorf("the detail surface was retargeted under the screen: loading %v, target %q", m.detail.IsLoading(), m.detail.TargetID())
	}
	if got := firstSelectionID(m, mode.Board); got != "tm-2" {
		t.Errorf("board selection: got %q, want tm-2", got)
	}
}

// A selection that landed under the screen is the one the issue keys act on,
// so the Detail the operator returns to shows that issue, not the one it
// showed when the screen opened.
func TestEscapeFromTheConfigScreenShowsTheSelectionThatLandedUnderIt(t *testing.T) {
	m, _ := configShell(t)
	m = press(t, m, "enter")
	if m.active != mode.Detail || m.detail.Detail.Summary.ID != "tm-1" {
		t.Fatalf("fixture: on %q showing %q, want detail on tm-1", m.active, m.detail.Detail.Summary.ID)
	}
	m = openConfigScreen(t, m)
	next, _ := m.Update(mode.SelectionChangedMsg{
		Mode:      mode.Board,
		Selection: &mode.Selection{Issue: domain.IssueSummary{ID: "tm-2", Title: "Triage inbox", Status: "open", Type: "task"}},
	})

	m = press(t, next.(Model), "esc")

	if m.active != mode.Detail {
		t.Fatalf("Escape landed on %q, want %q", m.active, mode.Detail)
	}
	if shown, selected := m.detail.Detail.Summary.ID, m.currentSelection().Issue.ID; shown != "tm-2" || selected != "tm-2" {
		t.Errorf("detail shows %q while the selection is %q, want tm-2 for both", shown, selected)
	}
}

// A change is written to the config file, applied to what every surface draws
// with, and kept in the config the app holds. Not parallel: it changes the
// styles.
func TestAChangeIsWrittenAppliedAndKept(t *testing.T) {
	restoreStyles(t)
	m, path := configShell(t)
	m = openConfigScreen(t, m)
	textBefore, boardBefore := styles.TextPrimaryColor, m.board.View(0)

	// Right of the last theme is the first.
	m = press(t, m, "right")
	if got := m.services.Config.UI; got.Theme != "catppuccin-frappe" || got.Glyphs != "unicode" {
		t.Fatalf("after right on the theme row the app holds %+v, want catppuccin-frappe and unicode", got)
	}
	if styles.TextPrimaryColor == textBefore {
		t.Error("the theme was not applied: the text colour is the one of catppuccin-mocha")
	}
	testui.AssertContainsAll(t, plainShell(m), "▌ theme     ‹ catppuccin-frappe ›")

	// Left of unicode is nerd, and Enter steps as Right does: ascii, after a
	// wrap.
	m = press(t, m, "down", "left")
	if got := m.services.Config.UI.Glyphs; got != "nerd" {
		t.Fatalf("after left on the glyph row the app holds %q, want nerd", got)
	}
	m = press(t, m, "enter", "enter")
	if got := m.services.Config.UI; got.Theme != "catppuccin-frappe" || got.Glyphs != "ascii" {
		t.Fatalf("after two steps on the glyph row the app holds %+v, want catppuccin-frappe and ascii", got)
	}
	if styles.Glyphs.Cursor != ">" {
		t.Errorf("the glyph set was not applied: the cursor is %q", styles.Glyphs.Cursor)
	}
	testui.AssertContainsAll(t, plainShell(m), "  theme     < catppuccin-frappe >", "> glyphs    < ascii >")
	if m.toast.Visible() {
		t.Errorf("a change that was written raised a toast:\n%s", plainShell(m))
	}

	// What the file holds is what the next start loads.
	loaded, err := config.LoadWithOptions(config.LoadOptions{Path: path, RequireExplicit: true})
	if err != nil {
		t.Fatalf("load the written config: %v", err)
	}
	if got := loaded.Config.UI; got.Theme != "catppuccin-frappe" || got.Glyphs != "ascii" {
		t.Errorf("the config file holds %+v, want catppuccin-frappe and ascii", got)
	}

	// The surface below draws with the new set once the screen is left.
	m = press(t, m, "esc")
	if m.active != mode.Board || m.board.View(0) == boardBefore {
		t.Errorf("on %q; the board is drawn as before the change: %v", m.active, m.board.View(0) == boardBefore)
	}
	testui.AssertContainsAll(t, plainShell(m), "> filter issues", "> T Fix login prompt")
}

// The screen is opened again on what is in use, not on what it showed last.
func TestConfigScreenReopensOnTheValuesInUse(t *testing.T) {
	restoreStyles(t)
	m, _ := configShell(t)
	m = openConfigScreen(t, m)
	m = press(t, m, "down", "right", "esc")
	if m.active != mode.Board || m.services.Config.UI.Glyphs != "ascii" {
		t.Fatalf("fixture: on %q with the glyph set %q", m.active, m.services.Config.UI.Glyphs)
	}

	m = openConfigScreen(t, m)
	testui.AssertContainsAll(t, plainShell(m), "> theme     < catppuccin-mocha >", "  glyphs    < ascii >")
}

// A write that fails changes nothing: not the styles, not the config the app
// holds, not what the screen shows. The operator is told in a toast.
func TestAFailedWriteChangesNothingAndShowsAToast(t *testing.T) {
	restoreStyles(t)

	notADir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	refused := filepath.Join(t.TempDir(), "config.yaml")
	flowStyle := "ui: {show_mode_switcher_help: true}\n"
	if err := os.WriteFile(refused, []byte(flowStyle), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "a path that cannot be written", path: filepath.Join(notADir, "config.yaml"), want: "not a directory"},
		{name: "a file Set refuses to edit", path: refused, want: "change it by hand"},
		{name: "no config path", path: "", want: "no config file to write to"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := configShell(t)
			m.services.ConfigPath = tc.path
			m = openConfigScreen(t, m)
			textBefore, cursorBefore := styles.TextPrimaryColor, styles.Glyphs.Cursor

			for _, keys := range [][]string{{"right"}, {"down", "left"}} {
				m = press(t, m, keys...)

				if got := m.services.Config.UI; got.Theme != "catppuccin-mocha" || got.Glyphs != "unicode" {
					t.Errorf("after %v the app holds %+v, want what it started with", keys, got)
				}
				if styles.TextPrimaryColor != textBefore || styles.Glyphs.Cursor != cursorBefore {
					t.Errorf("after %v the styles changed: text %v, cursor %q", keys, styles.TextPrimaryColor, styles.Glyphs.Cursor)
				}
				view := plainShell(m)
				// The toast wraps the reason, so it is read with the frame
				// and the line breaks taken out.
				told := strings.Join(strings.Fields(strings.ReplaceAll(view, "│", " ")), " ")
				if !m.toast.Visible() || !strings.Contains(told, "Config not changed:") || !strings.Contains(told, tc.want) {
					t.Errorf("after %v: toast visible %v, want one naming %q:\n%s", keys, m.toast.Visible(), tc.want, view)
				}
				testui.AssertContainsAll(t, view, "‹ catppuccin-mocha ›", "‹ unicode ›")
			}

			if tc.path == refused {
				if got, err := os.ReadFile(refused); err != nil || string(got) != flowStyle {
					t.Errorf("the refused file reads %q (%v), want it untouched", got, err)
				}
			}
		})
	}
}
