package app

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/helpscreen"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// newHelpShell is a loaded shell on a terminal of 120 columns and height rows
// with the help screen open.
func newHelpShell(t *testing.T, cfg config.Model, height int) Model {
	t.Helper()
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready", "task", 1)
	services, err := NewServices(gw, cfg, t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: height})
	if m = pressKey(t, m, "alt+h"); !m.showHelp {
		t.Fatal("fixture: the help screen did not open")
	}
	return m
}

// screenLines is the lines of the screen inside the margin, without colour.
func screenLines(m Model) []string {
	return strings.Split(testui.AnsiEscapePattern.ReplaceAllString(m.screen(), ""), "\n")
}

// helpBody is the lines of the help screen between the rule under its
// subtitle and its legend.
func helpBody(m Model) []string {
	lines := screenLines(m)
	return lines[4 : len(lines)-1]
}

// The help screen stands in the surface's place, whatever the surface: the
// title in the place of the menu bar, the rule, the subtitle over a second
// rule, the sections, and its own legend where the footer was. Nothing of the surface below is drawn.
func TestHelpIsAFullScreenOverEverySurface(t *testing.T) {
	for name, open := range map[string][]string{
		"board":        nil,
		"docs":         {"tab"},
		"search":       {"alt+f"},
		"detail":       {"enter"},
		"store picker": {"alt+s"},
		"config":       {"alt+c"},
	} {
		t.Run(name, func(t *testing.T) {
			m := newHelpShell(t, config.Default(), 60)
			m = press(t, pressKey(t, m, "alt+h"), open...)
			below := m.active
			m = pressKey(t, m, "alt+h")
			if !m.showHelp || m.active != below {
				t.Fatalf("help %v on %q, want help open over %q", m.showHelp, m.active, below)
			}

			lines := screenLines(m)
			if len(lines) != m.height {
				t.Fatalf("the help screen is %d lines on a screen of %d", len(lines), m.height)
			}
			if !strings.HasPrefix(lines[0], " Keyboard Help") || !strings.HasSuffix(lines[0], "dev") {
				t.Errorf("the first line is not the title and the version: %q", lines[0])
			}
			if lines[1] != " "+strings.Repeat("─", m.width-1) {
				t.Errorf("the second line is not the rule, one cell in: %q", lines[1])
			}
			if lines[2] != " every key taskmgr-ui answers to" {
				t.Errorf("the third line is not the subtitle: %q", lines[2])
			}
			if lines[3] != strings.Repeat("─", m.width) {
				t.Errorf("the fourth line is not the rule from edge to edge: %q", lines[3])
			}
			if !strings.HasPrefix(lines[4], " Moving and opening ─") {
				t.Errorf("the fifth line is not the first section: %q", lines[4])
			}
			if got := lines[len(lines)-1]; got != " down/up scroll • esc back • pgup/pgdown page • home/end bounds" {
				t.Errorf("the last line is not the help legend: %q", got)
			}
			for _, absent := range []string{"╭", "╰", "│", "❯", "written to"} {
				if strings.Contains(strings.Join(lines, "\n"), absent) {
					t.Errorf("%q of the surface below is drawn on the help screen", absent)
				}
			}
			for _, line := range lines {
				if lipgloss.Width(line) > m.width {
					t.Errorf("a line is %d cells on a screen of %d: %q", lipgloss.Width(line), m.width, line)
				}
			}

			if m = pressKey(t, m, "alt+h"); m.showHelp || m.active != below {
				t.Errorf("after the help key: help %v on %q, want %q without help", m.showHelp, m.active, below)
			}
		})
	}
}

// The sections stand in the order a new operator needs them, and each key is
// the one the resolved bindings give its action.
func TestHelpSectionsReadTheResolvedBindings(t *testing.T) {
	t.Parallel()

	keys, err := config.ResolveKeyBindings(config.Default().KeyBindings)
	if err != nil {
		t.Fatalf("ResolveKeyBindings: %v", err)
	}
	sections := helpSections(keys)

	var titles []string
	entries := map[string]string{}
	for _, section := range sections {
		titles = append(titles, section.Title)
		for _, entry := range section.Entries {
			if entry.Key == "" || entry.Desc == "" {
				t.Errorf("section %q has an entry without a key or a description: %+v", section.Title, entry)
			}
			entries[section.Title+": "+entry.Desc] = entry.Key
		}
	}
	want := "Moving and opening, Filter and search, Tabs, Issue, Detail and launchers, Menu bar and stores, Dialogs, Configuration screen, Mouse"
	if got := strings.Join(titles, ", "); got != want {
		t.Errorf("sections:\n got %s\nwant %s", got, want)
	}

	for entry, key := range map[string]string{
		"Moving and opening: the row above / below":                                         "up / down",
		"Moving and opening: open the selected issue in Detail":                             "enter",
		"Filter and search: search the store: titles, IDs and descriptions":                 "alt+f",
		"Tabs: next tab: Board, Docs":                                                       "tab/ctrl+pgdown",
		"Issue: close the selected issue":                                                   "delete",
		"Detail and launchers: launch nvim on the issue, in the background":                 "alt+v",
		"Menu bar and stores: open the configuration screen: the theme and the glyph set":   "alt+c",
		"Menu bar and stores: show and hide this screen":                                    "alt+h",
		"Mouse: select a box of text; letting go sends it to the terminal clipboard":        "drag",
		"Filter and search: in the search, switch between open issues and all of them":      "ctrl+t",
		"Filter and search: filter the rows of a tab by title or ID; every word must match": "type",
		"Dialogs: the next field or button":                                                 "tab/down",
		"Dialogs: confirm, while a button has the focus":                                    "y",
		"Dialogs: cancel the dialog":                                                        "esc",
		"Configuration screen: the setting above / below":                                   "up / down",
		"Configuration screen: the value before / after the one shown":                      "left / right",
	} {
		if entries[entry] != key {
			t.Errorf("%q has the key %q, want %q", entry, entries[entry], key)
		}
	}
}

// TestHelpNamesEveryBoundAction binds every action of every context to a key
// of its own and looks for each key in the sections: an action added to
// internal/config/keybindings.go without an entry here fails it, so the
// subtitle's "every key" stays true.
func TestHelpNamesEveryBoundAction(t *testing.T) {
	t.Parallel()

	bindings := config.DefaultKeyBindings()
	contexts := map[string]map[string][]string{
		config.ShellContext:  bindings.Shell,
		config.BoardContext:  bindings.Board,
		config.DetailContext: bindings.Detail,
		config.ModalContext:  bindings.Modal,
	}
	for context, actions := range contexts {
		for action := range actions {
			actions[action] = []string{"key-of-" + context + "-" + action}
		}
	}
	keys, err := config.ResolveKeyBindings(bindings)
	if err != nil {
		t.Fatalf("ResolveKeyBindings: %v", err)
	}

	named := map[string]bool{}
	for _, section := range helpSections(keys) {
		for _, entry := range section.Entries {
			for _, key := range strings.Split(entry.Key, " / ") {
				named[key] = true
			}
		}
	}
	for context, actions := range contexts {
		if len(actions) == 0 {
			t.Errorf("fixture: the %s context has no action", context)
		}
		for action, bound := range actions {
			if !named[bound[0]] {
				t.Errorf("no help entry names the key of %s in the %s context", action, context)
			}
		}
	}
}

// A rebound action shows its new key on the screen, and no entry keeps the
// default: the screen is read from the bindings on every draw.
func TestHelpShowsAReboundKey(t *testing.T) {
	cfg := config.Default()
	cfg.KeyBindings = config.MergeKeyBindings(cfg.KeyBindings, &config.KeyBindingOverride{
		Shell: map[string][]string{config.ShellActionHelp: {"f1"}, config.ShellActionOpenSearch: {"f3"}},
	})
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready", "task", 1)
	services, err := NewServices(gw, cfg, t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 60})

	// Close through the action that opens it, not through a literal key.
	if m = send(t, m, tea.KeyMsg{Type: tea.KeyF1}); !m.showHelp {
		t.Fatal("the rebound key did not open the help screen")
	}
	view := plainShell(m)
	for _, want := range []string{"f1 ", "f3 "} {
		if !strings.Contains(view, "   "+want) {
			t.Errorf("the help screen does not show the rebound key %q:\n%s", want, view)
		}
	}
	for _, gone := range []string{"alt+h", "alt+f"} {
		if strings.Contains(view, gone) {
			t.Errorf("the help screen still shows the default key %q:\n%s", gone, view)
		}
	}
	if m = send(t, m, tea.KeyMsg{Type: tea.KeyF1}); m.showHelp {
		t.Fatal("the rebound key did not close the help screen")
	}
}

// Escape closes the help screen and does nothing else: the surface below keeps
// its query and its place.
func TestEscapeClosesHelpAndLeavesTheSurfaceBelow(t *testing.T) {
	m := newHelpShell(t, config.Default(), 40)
	m = press(t, pressKey(t, m, "alt+h"), "enter", "alt+h")
	if !m.showHelp || m.active != mode.Detail {
		t.Fatalf("fixture: help %v on %q, want help over detail", m.showHelp, m.active)
	}

	if m = send(t, m, tea.KeyMsg{Type: tea.KeyEsc}); m.showHelp || m.active != mode.Detail {
		t.Fatalf("after Escape: help %v on %q, want detail without help", m.showHelp, m.active)
	}
}

// While the help screen is up it holds the keyboard: a key that is neither
// its own nor a scroll key reaches nothing below it, and changes nothing on
// it.
func TestHelpHoldsEveryOtherKey(t *testing.T) {
	m := newHelpShell(t, config.Default(), 60)
	whole := m.View()

	for _, key := range []string{"enter", "tab", "alt+s", "alt+c", "alt+f", "alt+n", "delete", "x", "left"} {
		next, cmd := m.Update(testKey(key))
		m = next.(Model)
		if cmd != nil {
			t.Errorf("%q under help returned a command", key)
		}
		if !m.showHelp || m.active != mode.Board || m.showActionModal || m.pendingDialog.active || m.View() != whole {
			t.Fatalf("%q under help: help %v, on %q, modal %v", key, m.showHelp, m.active, m.showActionModal)
		}
	}
}

// TestTheDetailScrollKeysScrollAClippedHelpScreen walks the help screen with
// the keyboard alone: a line, a page, both ends, and out.
func TestTheDetailScrollKeysScrollAClippedHelpScreen(t *testing.T) {
	m := newHelpShell(t, config.Default(), 16)
	top := m.View()
	topBody := helpBody(m)
	rows := styles.ScreenBodyRows(m.height)
	all := helpAllLines(t, m)
	if len(topBody) != rows || len(all) <= rows || topBody[0] != all[0] {
		t.Fatalf("fixture: the help screen must open clipped, on its first line:\n%s", top)
	}

	// The first and the last row count the lines beyond the window, as the
	// indicators of a detail pane do.
	if last := topBody[rows-1]; last != fmt.Sprintf(" … (%d more)", len(all)-rows) {
		t.Fatalf("the last row of the clipped help is %q, want the count of the lines below", last)
	}
	down := pressKey(t, pressKey(t, pressKey(t, m, "down"), "down"), "down")
	if body := helpBody(down); !down.showHelp || body[0] != " … (3 earlier)" || body[1] != all[4] {
		t.Fatalf("three presses of down drew %q and %q first, want the indicator of 3 lines and %q", body[0], body[1], all[4])
	}
	if up := send(t, down, tea.KeyMsg{Type: tea.KeyUp}); helpBody(up)[0] != " … (2 earlier)" || helpBody(up)[1] != all[3] {
		t.Fatal("up did not scroll the help screen back one line")
	}
	// The chrome stays where it is.
	for _, row := range []int{0, 1, 2, 3, m.height - 1} {
		if got, want := screenLines(down)[row], screenLines(m)[row]; got != want {
			t.Errorf("row %d moved with the scroll:\n got %q\nwant %q", row, got, want)
		}
	}

	page := send(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	// A page starts on the line after the last one the page before drew:
	// the indicators took the first and the last row.
	if topBody[rows-2] != all[rows-2] || helpBody(page)[1] != all[rows-1] {
		t.Fatalf("pgdown drew %q first, want the line after %q, %q", helpBody(page)[1], topBody[rows-2], all[rows-1])
	}
	if back := send(t, page, tea.KeyMsg{Type: tea.KeyPgUp}); back.View() != top {
		t.Fatal("pgup did not move the help screen back one page")
	}

	end := send(t, m, tea.KeyMsg{Type: tea.KeyEnd})
	if body := helpBody(end); body[len(body)-1] != all[len(all)-1] {
		t.Fatalf("end drew %q last, want the last line of the help, %q", body[len(body)-1], all[len(all)-1])
	}
	// Past the end there is nowhere to go, by a line, a page or the wheel.
	for name, further := range map[string]tea.Msg{
		"down":   tea.KeyMsg{Type: tea.KeyDown},
		"pgdown": tea.KeyMsg{Type: tea.KeyPgDown},
		"wheel":  wheel(10, 5, tea.MouseButtonWheelDown),
	} {
		if got := send(t, end, further); got.View() != end.View() {
			t.Errorf("%s at the end moved the help screen", name)
		}
	}
	if home := send(t, end, tea.KeyMsg{Type: tea.KeyHome}); home.View() != top {
		t.Fatal("home did not return the help screen to its first view")
	}
	if got := send(t, m, tea.KeyMsg{Type: tea.KeyUp}); got.View() != top {
		t.Error("up at the top moved the help screen")
	}

	// Closing and reopening starts at the top, by either way out.
	for name, closeKey := range map[string]tea.KeyMsg{
		"help key": testKey("alt+h"),
		"escape":   {Type: tea.KeyEsc},
	} {
		closed := send(t, end, closeKey)
		if closed.showHelp {
			t.Fatalf("%s did not close a scrolled help screen", name)
		}
		if reopened := pressKey(t, closed, "alt+h"); reopened.View() != top {
			t.Fatalf("the help screen closed with the %s reopened where it was scrolled to", name)
		}
	}
}

// TestEveryHelpLineIsReachedOnANarrowScreen walks the help down a line at a
// time on a terminal of 80 columns, where descriptions are wrapped: no line
// ends past the screen, and every line of the sections is drawn at some
// offset, the last one on the last row.
func TestEveryHelpLineIsReachedOnANarrowScreen(t *testing.T) {
	m := newHelpShell(t, config.Default(), 24)
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	all := helpAllLines(t, m)
	wrapped := false
	for _, line := range all {
		wrapped = wrapped || strings.HasPrefix(line, strings.Repeat(" ", 20)) && strings.TrimSpace(line) != ""
		if lipgloss.Width(line) > m.width-1 && !strings.Contains(line, "─") {
			t.Errorf("a line of the help ends on or past the last cell: %q", line)
		}
	}
	if !wrapped {
		t.Fatal("fixture: no description is wrapped at 80 columns")
	}

	drawn := map[string]bool{}
	for step := 0; step <= len(all); step++ {
		for _, line := range helpBody(m) {
			drawn[line] = true
		}
		m = pressKey(t, m, "down")
	}
	for _, line := range all {
		if !drawn[line] {
			t.Errorf("no offset draws the line %q", line)
		}
	}
	if body := helpBody(m); body[len(body)-1] != all[len(all)-1] || !strings.HasSuffix(body[0], " earlier)") {
		t.Errorf("the end of the help is %q under %q, want the last line under an indicator", body[len(body)-1], body[0])
	}

	// A wider terminal wraps fewer lines, and the offset follows.
	wide := send(t, m, tea.WindowSizeMsg{Width: 140, Height: 24})
	if body, lines := helpBody(wide), helpAllLines(t, wide); len(lines) >= len(all) || body[len(body)-1] != lines[len(lines)-1] {
		t.Errorf("after the resize the last body row is %q, want the last line of the help", body[len(body)-1])
	}
}

// helpAllLines is every line of the help sections at m's width, without
// colour: what a screen tall enough for all of them draws.
func helpAllLines(t *testing.T, m Model) []string {
	t.Helper()

	m.helpOffset = 0
	m.height = helpscreen.MaxOffset(helpSections(m.keys), m.width, 0) + 5
	return helpBody(m)
}

// TestTheHelpScreenScrollsOnReboundDetailKeys: the keys come from the detail
// context of the resolved bindings, not from literals.
func TestTheHelpScreenScrollsOnReboundDetailKeys(t *testing.T) {
	cfg := config.Default()
	cfg.KeyBindings = config.MergeKeyBindings(cfg.KeyBindings, &config.KeyBindingOverride{
		Detail: map[string][]string{config.DetailActionScrollDown: {"alt+z"}},
	})
	m := newHelpShell(t, cfg, 16)
	top := m.View()

	if still := pressKey(t, m, "down"); still.View() != top || !still.showHelp {
		t.Fatal("a key no longer bound to detail scroll_down scrolled or closed the help screen")
	}
	if down := pressKey(t, m, "alt+z"); helpBody(down)[1] != helpBody(m)[2] {
		t.Fatalf("the rebound scroll_down key did not scroll the help screen:\n%s", down.View())
	}
}

// TestTheScrollKeysChangeNothingWhenTheHelpFits: in a terminal tall enough for
// the whole help there is nowhere to scroll to.
func TestTheScrollKeysChangeNothingWhenTheHelpFits(t *testing.T) {
	m := newHelpShell(t, config.Default(), 80)
	whole := m.View()
	if !strings.Contains(whole, "shift+drag") || !strings.Contains(whole, "Moving and opening") {
		t.Fatalf("fixture: the help screen is clipped at this height:\n%s", whole)
	}
	for _, msg := range []tea.Msg{
		tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyUp},
		tea.KeyMsg{Type: tea.KeyPgDown}, tea.KeyMsg{Type: tea.KeyPgUp},
		tea.KeyMsg{Type: tea.KeyEnd}, tea.KeyMsg{Type: tea.KeyHome},
		wheel(10, 5, tea.MouseButtonWheelDown),
	} {
		if m = send(t, m, msg); m.View() != whole || !m.showHelp {
			t.Fatalf("%v changed a help screen that fits", msg)
		}
	}
}

// TestTheWheelSkipsNoHelpLineOnAShortScreen: a body of three or four rows
// draws one or two lines between its indicators, fewer than a notch scrolled.
func TestTheWheelSkipsNoHelpLineOnAShortScreen(t *testing.T) {
	for _, height := range []int{8, 9, 10, 16} {
		m := newHelpShell(t, config.Default(), height)
		all := helpAllLines(t, m)

		drawn := map[string]bool{}
		for range all {
			for _, line := range helpBody(m) {
				drawn[line] = true
			}
			m = send(t, m, wheel(10, 5, tea.MouseButtonWheelDown))
		}
		for _, line := range all {
			if !drawn[line] {
				t.Errorf("height %d: the wheel draws the line %q at no notch", height, line)
			}
		}
	}
}

// TestHelpNamesTheKeysDetailTakesItself: Detail answers to three keys no
// binding names, which TestHelpNamesEveryBoundAction cannot see.
func TestHelpNamesTheKeysDetailTakesItself(t *testing.T) {
	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		t.Fatalf("ResolveKeyBindings: %v", err)
	}
	for _, section := range helpSections(keys) {
		if section.Title != "Detail and launchers" {
			continue
		}
		var got []string
		for _, entry := range section.Entries {
			if entry.Key == "left / right" || entry.Key == "enter" {
				got = append(got, entry.Key+": "+entry.Desc)
			}
		}
		want := []string{
			"left / right: the pane to the left / right",
			"enter: in the Dependencies pane, open the selected issue",
			"enter: in the Metadata pane, change the selected Status or Priority",
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("the Detail section names\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
		return
	}
	t.Fatal("no Detail section in the help")
}

// A resize under a scrolled help screen leaves no empty rows below its last
// line, and the next key moves from the line that is drawn, not from an offset
// the larger screen left behind.
func TestTheHelpScrollHoldsAfterAResize(t *testing.T) {
	m := newHelpShell(t, config.Default(), 16)
	all := helpAllLines(t, m)
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnd})

	taller := send(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	body := helpBody(taller)
	if len(body) != styles.ScreenBodyRows(taller.height) || body[len(body)-1] != all[len(all)-1] {
		t.Fatalf("after the resize the last body row is %q, want the last line of the help %q", body[len(body)-1], all[len(all)-1])
	}
	// One line up, the second row is the line the first row drew before: the
	// first row is the indicator's.
	if up := pressKey(t, taller, "up"); helpBody(up)[1] != all[len(all)-len(body)] {
		t.Errorf("up after the resize drew %q on its second row, want %q", helpBody(up)[1], all[len(all)-len(body)])
	}

	shorter := send(t, taller, tea.WindowSizeMsg{Width: 120, Height: 12})
	if got := len(strings.Split(shorter.View(), "\n")); got != 12 {
		t.Errorf("the help screen is %d lines on a terminal of 12", got)
	}
	if end := send(t, shorter, tea.KeyMsg{Type: tea.KeyEnd}); helpBody(end)[len(helpBody(end))-1] != all[len(all)-1] {
		t.Error("end after the second resize does not draw the last line of the help")
	}
}

// The help screen holds no text of an older theme or glyph set: it is drawn
// from the roles on every frame, and it scrolls on after the change.
func TestTheHelpScreenFollowsAThemeChange(t *testing.T) {
	t.Cleanup(func() {
		if err := styles.Apply("catppuccin-mocha", "unicode"); err != nil {
			t.Fatalf("restore the initial styles: %v", err)
		}
	})
	testui.ForceTrueColor(t)

	m := newHelpShell(t, config.Default(), 16)
	m = pressKey(t, m, "down")
	mocha := m.View()

	if err := styles.Apply("catppuccin-latte", "ascii"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	latte := m.View()
	if latte == mocha {
		t.Fatal("the help screen is drawn in the colours of the theme before")
	}
	heading := lipgloss.NewStyle().Foreground(styles.SectionHeadingColor).Bold(true).Render("Filter and search")
	if !strings.Contains(latte, heading) {
		t.Errorf("a section heading is not in the heading colour of the new theme:\n%q", latte)
	}
	if plainShell(m) != testui.AnsiEscapePattern.ReplaceAllString(mocha, "") {
		t.Error("the theme change moved the text of the help screen")
	}
	if down := pressKey(t, m, "down"); helpBody(down)[1] != helpBody(m)[2] {
		t.Error("down after the theme change did not scroll the help screen one line")
	}
}
