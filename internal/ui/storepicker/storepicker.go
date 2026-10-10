// Package storepicker renders the store picker: the full-screen list of every
// central task-manager store on this machine.
//
// It renders instead of the shell rather than inside it, so it draws its own
// help line where the shell footer would be.
package storepicker

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/ui/loading"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

const (
	defaultWidth  = 100
	defaultHeight = 24

	// helpLines is the row the help hint occupies below the section box.
	helpLines = 1

	// rowLines is the number of lines one row draws: the store's name and its
	// status, then its project path.
	rowLines = 2
)

// Row is one list entry: a registry entry, or an action when Action is set.
type Row struct {
	// Action is the label of a row that does something rather than naming a
	// store — "Create a local store in …". An action row carries no other
	// field.
	Action string

	Name        string
	ProjectPath string
	// Health is the entry's health token, rendered when the entry is not
	// usable. An unusable entry stays on the list: hiding it would disagree
	// with what `taskmgr store list` shows.
	Health string
	Usable bool
	// Active marks the store the app is currently browsing.
	Active bool
}

// State is the full picker renderer input.
type State struct {
	Rows         []Row
	SelectedRow  int
	ScrollOffset int
	// Loading is true while a listing is in flight. With no rows yet this
	// draws skeleton rows; with rows already on screen they stay, and the
	// spinner in the section title carries the in-flight state.
	Loading bool
	// Error is non-empty when the listing failed. It pins an inline row above
	// the entries, which keeps any stale rows readable.
	Error string
	// Help is the key legend drawn below the box, in place of the shell
	// footer. It arrives styled (styles.KeyLegend).
	Help         string
	SpinnerFrame int
	Width        int
	Height       int
	// Hover is the index of the row under the pointer, as HitTest reported it;
	// nil when the pointer is elsewhere. That row draws the hover band.
	Hover *int
}

// HitTest reports the index of the row Render(state) draws at cell (x, y),
// with (0, 0) the first cell of the frame. ok is false anywhere but on a row.
func HitTest(state State, x, y int) (row int, ok bool) {
	width := state.Width
	if width <= 0 {
		width = defaultWidth
	}
	height := state.Height
	if height <= 0 {
		height = defaultHeight
	}

	line := y - 1
	if state.Error != "" {
		line--
	}
	hasError := state.Error != ""
	// A frame too short for one whole row still draws its first line, so the
	// rows end where the content does.
	rowsEnd := min(rowLines*RowCapacity(height, hasError), contentLines(height, hasError))
	if x < 1 || x >= width-1 || line < 0 || line >= rowsEnd {
		return 0, false
	}
	row = max(0, min(state.ScrollOffset, len(state.Rows))) + line/rowLines
	if row >= len(state.Rows) {
		return 0, false
	}
	return row, true
}

// RowCapacity returns how many store rows fit at this terminal height. The
// controller derives its scroll window from it, so the window and the rows
// actually drawn cannot disagree.
func RowCapacity(height int, hasError bool) int {
	if height <= 0 {
		height = defaultHeight
	}
	rows := contentLines(height, hasError) / rowLines
	if rows < 1 {
		rows = 1
	}
	return rows
}

// contentLines is the number of lines inside the box below the error row.
func contentLines(height int, hasError bool) int {
	// Two border lines plus the help line below the box.
	lines := height - 2 - helpLines
	if hasError {
		lines--
	}
	return max(lines, 1)
}

// Render draws the picker.
func Render(state State) string {
	width := state.Width
	if width <= 0 {
		width = defaultWidth
	}
	height := state.Height
	if height <= 0 {
		height = defaultHeight
	}

	innerWidth := width - 2
	if innerWidth < 1 {
		innerWidth = 1
	}
	capacity := RowCapacity(height, state.Error != "")

	content := make([]string, 0, capacity+1)
	if state.Error != "" {
		content = append(content, renderError(state.Error, innerWidth))
	}
	content = append(content, renderRows(state, innerWidth, capacity, contentLines(height, state.Error != ""))...)

	box := styles.FormSection(styles.FormSectionConfig{
		Content:            content,
		Width:              width,
		Height:             height - helpLines,
		TopLeft:            title(state),
		TopRight:           countLabel(state, capacity),
		Focused:            true,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	help := textutil.TruncateString(state.Help, width)

	return lipgloss.JoinVertical(lipgloss.Left, box, help)
}

func title(state State) string {
	if state.Loading {
		return "Task Stores " + loading.Glyph(state.SpinnerFrame)
	}
	return "Task Stores"
}

// countLabel follows the header-count rule: a plain N only when the whole list
// is loaded and fits, "N of M" for a clipped window, and the skeleton glyph
// while a cold listing is in flight. It counts stores; the create rows are not
// stores.
func countLabel(state State, capacity int) string {
	total := storeRows(state.Rows)
	if state.Loading && total == 0 {
		return issuerow.SkeletonGlyph
	}
	if len(state.Rows) <= capacity {
		return fmt.Sprintf("%d", total)
	}
	offset := max(0, min(state.ScrollOffset, len(state.Rows)))
	visible := storeRows(state.Rows[offset:min(offset+capacity, len(state.Rows))])
	return fmt.Sprintf("%d of %d", visible, total)
}

func storeRows(rows []Row) int {
	n := 0
	for _, row := range rows {
		if row.Action == "" {
			n++
		}
	}
	return n
}

// renderRows draws capacity rows into exactly lines lines.
func renderRows(state State, innerWidth, capacity, lines int) []string {
	if state.Loading && len(state.Rows) == 0 {
		return skeletonRows(innerWidth, lines)
	}
	if len(state.Rows) == 0 {
		// With an error above them, no rows means the read failed, not that the
		// registry is empty. Saying "no stores are registered" here is the one
		// wrong answer, so the error line is left to speak alone.
		if state.Error != "" {
			return blankRows(lines)
		}
		return emptyState(innerWidth, lines)
	}

	offset := state.ScrollOffset
	if offset < 0 {
		offset = 0
	}
	if offset > len(state.Rows) {
		offset = len(state.Rows)
	}
	visible := state.Rows[offset:]
	if len(visible) > capacity {
		visible = visible[:capacity]
	}

	out := make([]string, 0, lines)
	for idx, row := range visible {
		hovered := state.Hover != nil && *state.Hover == offset+idx
		selected := offset+idx == state.SelectedRow
		for _, line := range renderRow(row, selected, innerWidth) {
			out = append(out, styles.RowHighlight(line, innerWidth, selected, hovered))
		}
	}
	for len(out) < lines {
		out = append(out, "")
	}
	return out[:lines]
}

// renderRow draws one row as rowLines lines: the name with its status token
// flush right, then the project path. An action row has no second line to
// fill; it keeps the height so every row is one size. Each text names a
// directory, so each goes through textutil.OneLine.
func renderRow(row Row, selected bool, innerWidth int) []string {
	row.Name = textutil.OneLine(row.Name)
	row.ProjectPath = textutil.OneLine(row.ProjectPath)
	row.Action = textutil.OneLine(row.Action)

	plainPrefix, renderedPrefix := styles.SelectionPrefix(selected, true)
	textWidth := max(innerWidth-lipgloss.Width(plainPrefix), 0)

	if row.Action != "" {
		label := textutil.TruncateString(row.Action, textWidth)
		return []string{
			renderedPrefix + lipgloss.NewStyle().Foreground(styles.TextSecondaryColor).Bold(true).Render(label),
			renderedPrefix,
		}
	}

	nameStyle := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor)
	if !row.Usable {
		nameStyle = lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	}

	token, tokenStyle := statusToken(row)
	tokenWidth := 0
	if token != "" {
		tokenWidth = lipgloss.Width(token) + 1
	}
	nameWidth := max(textWidth-tokenWidth, 1)

	first := renderedPrefix + nameStyle.Render(textutil.PadToWidth(textutil.TruncateString(row.Name, nameWidth), nameWidth))
	if token != "" {
		first += " " + tokenStyle.Render(token)
	}
	path := lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render(textutil.TruncateString(row.ProjectPath, textWidth))

	return []string{textutil.TruncateString(first, innerWidth), renderedPrefix + path}
}

// statusToken returns the one token a row may carry on its right.
//
// Health outranks "active". A store can be opened and then have its directory
// removed from under the running app, and that row is the one the operator most
// needs told: reporting it as a healthy active store is the one answer that
// helps nobody.
func statusToken(row Row) (string, lipgloss.Style) {
	if !row.Usable {
		style := lipgloss.NewStyle().Foreground(styles.StoreBrokenColor)
		if row.Health == "dangling" {
			style = lipgloss.NewStyle().Foreground(styles.StoreDanglingColor)
		}
		return row.Health, style
	}
	if row.Active {
		return "active", lipgloss.NewStyle().Foreground(styles.StoreActiveColor).Bold(true)
	}
	return "", lipgloss.NewStyle()
}

func blankRows(capacity int) []string {
	out := make([]string, 0, capacity)
	for i := 0; i < capacity; i++ {
		out = append(out, "")
	}
	return out
}

func skeletonRows(innerWidth, capacity int) []string {
	out := make([]string, 0, capacity)
	for i := 0; i < capacity; i++ {
		out = append(out, "")
	}
	if capacity > 0 {
		out[0] = lipgloss.NewStyle().
			Foreground(styles.TextMutedColor).
			Render(textutil.TruncateString("Reading the central store registry…", innerWidth))
	}
	return out
}

// emptyState names what would be listed here rather than leaving a bare frame,
// because an empty registry and a failed read look identical otherwise.
func emptyState(innerWidth, capacity int) []string {
	lines := []string{
		"No central task-manager stores are registered on this machine.",
		"",
		"A store created with 'taskmgr init --central' is listed here.",
		"A local .tasks store is not: open it by starting taskmgr-ui in its project.",
	}

	out := make([]string, 0, capacity)
	style := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	for _, line := range lines {
		if len(out) >= capacity {
			break
		}
		if line == "" {
			out = append(out, "")
			continue
		}
		out = append(out, style.Render(textutil.TruncateString(line, innerWidth)))
	}
	for len(out) < capacity {
		out = append(out, "")
	}
	return out
}

// renderError draws the failure on one line. The message is the SDK's own:
// yaml.v3 reports a type error in the registry file on a line for each value.
func renderError(message string, innerWidth int) string {
	text := textutil.TruncateString("Store listing failed: "+textutil.OneLine(message), innerWidth)
	return lipgloss.NewStyle().Foreground(styles.ToastBorderErrorColor).Render(text)
}
