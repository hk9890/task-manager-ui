package app

import (
	"errors"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	configscreenmode "github.com/hk9890/task-manager-ui/internal/mode/configscreen"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

// toastMargin is the cells a toast spends beside its text: the border, the
// padding and the severity glyph.
const toastMargin = 8

// applyConfigChange makes a theme and a glyph set the ones in use: checked,
// written to the config file, then applied. The file comes first, so a write
// that fails changes nothing and the screen keeps showing what is drawn.
//
// It runs in Update and not in a command: styles.Apply assigns the package
// variables every renderer reads, and View reads them on this goroutine.
func (m *Model) applyConfigChange(change configscreenmode.ChangeMsg) tea.Cmd {
	err := styles.Validate(change.Theme, change.Glyphs)
	if err == nil {
		err = m.writeUIConfig(change)
	}
	if err == nil {
		err = styles.Apply(change.Theme, change.Glyphs)
	}
	if err != nil {
		m.logger().Error("failed to change the ui configuration", "path", m.services.ConfigPath, "error", err.Error())
		// The toast cuts each line at the terminal width, and the error ends
		// in what the operator is to do, after the path of the file.
		reason := textutil.WrapLines(err.Error(), max(m.width-toastMargin, 1))
		return m.showToast("Config not changed:\n"+strings.Join(reason, "\n"), toaster.StyleError)
	}

	m.services.Config.UI.Theme = change.Theme
	m.services.Config.UI.Glyphs = change.Glyphs
	m.configScreen.SetValues(change.Theme, change.Glyphs)
	// A light theme renders markdown in another glamour style and a glyph set
	// draws tokens of another width, so the text under a stored scroll offset
	// may be shorter now. A resize clamps the offsets, and so does this.
	m.applyWorkspaceSizeToBrowseModes()
	m.detail.ClampScroll(m.detailViewportWidth(), m.detailViewportHeight())
	return nil
}

// writeUIConfig writes each value that differs from the one in use to the
// config file.
func (m *Model) writeUIConfig(change configscreenmode.ChangeMsg) error {
	path, ui := m.services.ConfigPath, m.services.Config.UI
	if path == "" {
		return errors.New("no config file to write to")
	}
	if change.Theme != ui.Theme {
		if err := config.Set(path, "ui", "theme", change.Theme); err != nil {
			return err
		}
	}
	if change.Glyphs != ui.Glyphs {
		return config.Set(path, "ui", "glyphs", change.Glyphs)
	}
	return nil
}
