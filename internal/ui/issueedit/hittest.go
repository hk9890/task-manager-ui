package issueedit

import (
	"charm.land/lipgloss/v2"

	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// Hit is the field drawn at a cell.
type Hit struct{ Field Field }

// HitTest reports the field Render draws at screen cell (x, y): a row, the
// text, or one of the two buttons. A blank line, a read-only row, the error
// and the chrome report nothing, and so do the buttons during a save.
func HitTest(state State, x, y int) (Hit, bool) {
	errors := len(errorLines(state))
	_, fields := form(state)
	rows := styles.ScreenBodyRows(state.Height) - errors
	line := y - styles.ScreenBodyTop - errors
	if x < 0 || x >= state.Width || line < 0 || line >= rows {
		return Hit{}, false
	}
	line += window(state, fields, rows)
	if line >= len(fields) || fields[line] == noField {
		return Hit{}, false
	}
	if fields[line] == FieldSave {
		return buttonAt(state, x)
	}
	return Hit{Field: fields[line]}, true
}

// buttonAt is the button the button line draws at column x.
func buttonAt(state State, x int) (Hit, bool) {
	if state.Saving {
		return Hit{}, false
	}
	save, cancel := buttons(state)
	saveEnd := gutterWidth + lipgloss.Width(save)
	cancelStart := saveEnd + buttonGap
	switch {
	case x >= gutterWidth && x < saveEnd:
		return Hit{Field: FieldSave}, true
	case x >= cancelStart && x < cancelStart+lipgloss.Width(cancel):
		return Hit{Field: FieldCancel}, true
	}
	return Hit{}, false
}
