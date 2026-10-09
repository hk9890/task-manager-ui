package styles

const (
	selectionSelectedPrefix = "› "
	selectionIdlePrefix     = "  "
)

// SelectionPrefix returns the shared 2-character selection gutter prefix.
// The plain variant is unstyled and should be used for width math/truncation.
// The rendered variant applies app-wide selection styling when requested.
func SelectionPrefix(selected, styled bool) (plain string, rendered string) {
	if !selected {
		return selectionIdlePrefix, selectionIdlePrefix
	}

	if styled {
		return selectionSelectedPrefix, SelectionIndicatorStyle.Render("›") + " "
	}

	return selectionSelectedPrefix, selectionSelectedPrefix
}

// RowPrefix is the gutter of a row that can be both selected and under the
// pointer. Selection wins: the hover chevron marks where a click would move the
// selection to, and it is already there.
func RowPrefix(selected, hovered, styled bool) (plain string, rendered string) {
	if selected || !hovered {
		return SelectionPrefix(selected, styled)
	}
	if styled {
		return selectionSelectedPrefix, HoverIndicatorStyle.Render("›") + " "
	}
	return selectionSelectedPrefix, selectionSelectedPrefix
}
