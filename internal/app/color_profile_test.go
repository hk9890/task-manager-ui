package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// The program reports the terminal's colour profile after the start, and the
// hover roles are chosen by it.
func TestAReportedColorProfileReappliesTheRoles(t *testing.T) {
	restoreStyles(t)
	m, _ := filterShell(t)
	ui := m.services.Config.UI

	if err := styles.Apply(ui.Theme, ui.Glyphs, colorprofile.ANSI256); err != nil {
		t.Fatal(err)
	}
	onPalette := styles.RowHoverBgColor
	if err := styles.Apply(ui.Theme, ui.Glyphs, colorprofile.TrueColor); err != nil {
		t.Fatal(err)
	}
	if onPalette == styles.RowHoverBgColor {
		t.Fatalf("fixture: the hover %v is the same on 256 colours and on true colour", onPalette)
	}

	m = send(t, m, tea.ColorProfileMsg{Profile: colorprofile.ANSI256})
	if m.runtime.ColorProfile != colorprofile.ANSI256 || styles.RowHoverBgColor != onPalette {
		t.Errorf("after a 256-colour report: profile %v and hover %v; want %v and %v",
			m.runtime.ColorProfile, styles.RowHoverBgColor, colorprofile.ANSI256, onPalette)
	}
}
