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
