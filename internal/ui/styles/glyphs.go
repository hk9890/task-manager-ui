package styles

import "strconv"

// GlyphSet is the marker vocabulary a surface draws with. A row marker — the
// cursor, a spinner frame, an issue token that is not letters — is one cell
// wide, whatever the set. A toast glyph may be two, as the success and error
// marks of the unicode set are: the toaster budgets for the wider one.
//
// The set covers the markers that carry meaning: the selection bar, the
// spinner, the query prompt, the toast severities and the issue vocabulary. Section borders, the
// `…` of truncated text and the skeleton bar are the same in every set.
type GlyphSet struct {
	// Cursor is the bar down the left of the selected row.
	Cursor string
	// Spinner is the frames of work in flight.
	Spinner []string
	// Prompt stands in front of a query line.
	Prompt string

	ToastSuccess string
	ToastError   string
	ToastInfo    string
	ToastWarn    string

	// issueType and status are keyed by the normalized token. A set that
	// leaves status empty spells a status with its letters.
	issueType        map[string]string
	issueTypeUnknown string
	// priority is P0 to P4, every priority the store accepts, so that no two
	// are drawn alike; a set that leaves it empty spells "P<n>".
	priority      []string
	status        map[string]string
	statusUnknown string
}

// Glyphs is the applied set. Apply assigns it.
var Glyphs GlyphSet

var brailleFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var letterTypes = map[string]string{
	"bug": "B", "task": "T", "feature": "F", "epic": "E", "chore": "C", "doc": "D",
}

// A toast glyph is a text-presentation form, carrying no U+FE0F variation
// selector. The emoji-presentation variants "ℹ️" and "⚠️" measure two cells
// under lipgloss.Width and one under wcwidth, so the frame was built a cell
// wider than the terminal drew it and overlay.Place spliced the line short.
// toaster.TestToastGlyphWidthsAgreeWithWcwidth pins the two measures together.
//
// The three sets. Nerd says an issue's type, priority and status with icons
// and needs a patched font. Unicode says them with letters and the characters
// any font a terminal ships with has. ASCII is for a terminal whose font is
// not yours.
//
// The cursor is a half block in every set but ASCII: a bar the height of the
// row reads as "this one" from across the screen, and on a two-line row it
// joins the lines into one.
var glyphSets = map[string]GlyphSet{
	"unicode": {
		Cursor:       "▌",
		Spinner:      brailleFrames,
		Prompt:       "❯",
		ToastSuccess: "✅", ToastError: "❌", ToastInfo: "ℹ", ToastWarn: "⚠",
		issueType: letterTypes, issueTypeUnknown: "?",
	},
	"nerd": {
		Cursor:       "▌",
		Spinner:      brailleFrames,
		Prompt:       "❯",
		ToastSuccess: "", // nf-fa-check
		ToastError:   "", // nf-fa-times
		ToastInfo:    "", // nf-fa-info
		ToastWarn:    "", // nf-fa-exclamation_triangle
		issueType: map[string]string{
			"bug":     "", // nf-fa-bug
			"task":    "", // nf-fa-tasks
			"feature": "", // nf-fa-star
			"epic":    "", // nf-fa-bolt
			"chore":   "", // nf-fa-wrench
			"doc":     "", // nf-fa-file_text
		},
		issueTypeUnknown: "", // nf-fa-question
		priority: []string{
			"", // nf-fa-angle_double_up
			"", // nf-fa-angle_up
			"", // nf-fa-minus
			"", // nf-fa-angle_down
			"", // nf-fa-angle_double_down
		},
		status: map[string]string{
			"open":        "", // nf-fa-circle_o
			"ready":       "", // nf-fa-circle
			"in_progress": "", // nf-fa-play
			"blocked":     "", // nf-fa-ban
			"closed":      "", // nf-fa-check
			"deferred":    "", // nf-fa-pause
		},
		statusUnknown: "", // nf-fa-question
	},
	"ascii": {
		Cursor: ">",
		Prompt: ">",
		// Five frames: the shell counts ten, and a cycle that divides it does
		// not jump where the count wraps.
		Spinner:      []string{".", "o", "O", "o", "."},
		ToastSuccess: "+", ToastError: "x", ToastInfo: "i", ToastWarn: "!",
		issueType: letterTypes, issueTypeUnknown: "?",
	},
}

// IssueType is the marker of an issue type. recognised is false for a type the
// set has no marker of its own for.
func (g GlyphSet) IssueType(issueType string) (glyph string, recognised bool) {
	token := normalizeIssueToken(issueType)
	if token == "docs" {
		token = "doc"
	}
	if glyph, ok := g.issueType[token]; ok {
		return glyph, true
	}
	return g.issueTypeUnknown, false
}

// Priority is the marker of a priority.
func (g GlyphSet) Priority(priority int) string {
	if priority < 0 {
		priority = 0
	}
	if len(g.priority) == 0 {
		return "P" + strconv.Itoa(priority)
	}
	return g.priority[min(priority, len(g.priority)-1)]
}

// Status is the icon of a status. ok is false in a set that spells a status
// with letters; the caller then derives the token.
func (g GlyphSet) Status(status string) (glyph string, ok bool) {
	if g.status == nil {
		return "", false
	}
	if glyph, found := g.status[normalizeIssueToken(status)]; found {
		return glyph, true
	}
	return g.statusUnknown, true
}
