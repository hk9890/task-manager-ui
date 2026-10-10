package app

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

// A drag of the left button selects a box of the screen and copies its text.
// The terminal's own selection cannot do it: the program asks for every mouse
// event, so the terminal selects only with shift held, and a refresh rewriting
// a line under that selection clears it.
//
// The box is drawn over the screen as it was when the drag began, and the text
// is taken from that screen, so a refresh landing mid-drag moves nothing under
// the pointer. The model keeps updating underneath and is shown again on
// release.
type textSelection struct {
	active   bool
	from, to screenCell
	screen   string
}

// screenCell is a cell of the screen, counted from its top left: the first
// cell inside the margin, not the terminal's.
type screenCell struct {
	x, y int
}

// dragFrom is how far, in cells either way, the pointer moves from the press
// before a drag begins. A hand that shifts one cell during a click is still
// clicking, and a selection there would overwrite the clipboard with two
// characters.
const dragFrom = 2

// box is the selection's corners, top left first, inclusive.
func (s textSelection) box() (left, top, right, bottom int) {
	return min(s.from.x, s.to.x), min(s.from.y, s.to.y), max(s.from.x, s.to.x), max(s.from.y, s.to.y)
}

// glyphEdges widens the cells [left, right] of a line without escape codes
// until neither end splits a glyph two cells wide. Cut through its middle, such
// a glyph is dropped on one side of the edge and kept whole on the other: the
// row is drawn a cell out of line and the copy disagrees with the box.
func glyphEdges(plain string, left, right int) (int, int) {
	for cell := 0; plain != "" && cell <= right; {
		cluster, width := ansi.FirstGraphemeCluster(plain, ansi.GraphemeWidth)
		last := cell + width - 1
		if cell < left && left <= last {
			left = cell
		}
		if cell <= right && right < last {
			right = last
		}
		plain = plain[len(cluster):]
		cell += width
	}
	return left, right
}

// text is what the box holds, one line per row, without colour and without
// the spaces a row is padded with.
func (s textSelection) text() string {
	left, top, right, bottom := s.box()
	lines := strings.Split(s.screen, "\n")
	var out []string
	for y := top; y <= bottom && y < len(lines); y++ {
		plain := ansi.Strip(lines[y])
		from, to := glyphEdges(plain, left, right)
		out = append(out, strings.TrimRight(ansi.Cut(plain, from, to+1), " "))
	}
	return strings.Join(out, "\n")
}

// view is the frozen screen with the box in reverse video. The styles of a
// line resume after the box, because the part after it keeps the escape codes
// of the part cut away.
func (s textSelection) view() string {
	left, top, right, bottom := s.box()
	lines := strings.Split(s.screen, "\n")
	for y := top; y <= bottom && y < len(lines); y++ {
		line := lines[y]
		plain := ansi.Strip(line)
		from, to := glyphEdges(plain, left, right)
		lines[y] = textutil.PadToWidth(ansi.Truncate(line, from, ""), from) +
			"\x1b[0;7m" + textutil.PadToWidth(ansi.Cut(plain, from, to+1), to-from+1) + "\x1b[0m" +
			ansi.TruncateLeft(line, to+1, "")
	}
	return strings.Join(lines, "\n")
}

// mouseHeld follows the left button from its press to its release. handled is
// true for an event that belongs to a drag, which nothing else may see: a
// wheel or a click during a selection would change the screen it stands over.
//
// A press only notes where the button went down and is then handled as a click
// as usual, so a row is selected before a drag off it begins. Motion with no
// button held says the release went to another window: the press is forgotten
// and nothing is copied.
func (m Model) mouseHeld(msg tea.MouseMsg) (next Model, cmd tea.Cmd, handled bool) {
	// A drag runs on into the margin, and the box stops at the screen's edge.
	// On a screen with no column or no row the box stops at cell 0: it finds
	// the lines it covers by its rows, and a row of -1 is no line.
	at := screenCell{x: textutil.Clamp(msg.X, 0, max(m.width-1, 0)), y: textutil.Clamp(msg.Y, 0, max(m.height-1, 0))}
	leftPress := msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft

	switch {
	case leftPress && !m.sel.active:
		m.press = &at
		return m, nil, false
	case m.press == nil:
		return m, nil, false
	case msg.Action == tea.MouseActionMotion && msg.Button == tea.MouseButtonLeft:
		if !m.sel.active {
			if max(abs(at.x-m.press.x), abs(at.y-m.press.y)) < dragFrom {
				return m, nil, true
			}
			m.sel = textSelection{active: true, from: *m.press, screen: m.screen()}
		}
		m.sel.to = at
		return m, nil, true
	case msg.Action == tea.MouseActionMotion && msg.Button == tea.MouseButtonNone:
		m.press, m.sel = nil, textSelection{}
		return m, nil, false
	case msg.Action == tea.MouseActionRelease:
		selected := m.sel
		m.press, m.sel = nil, textSelection{}
		if !selected.active {
			return m, nil, false
		}
		// A box of blank cells copies nothing: an empty copy clears the
		// clipboard.
		text := selected.text()
		if strings.TrimSpace(text) == "" {
			return m, nil, true
		}
		count := len([]rune(strings.ReplaceAll(text, "\n", "")))
		// OSC 52 has no reply: the toast says what was sent, not that it arrived.
		toast := m.showToast(fmt.Sprintf("Sent %d characters to the clipboard; shift+drag if not copied", count), toaster.StyleInfo)
		return m, batchCmds(m.copyText(text), toast), true
	}
	return m, nil, m.sel.active
}

// selectingKey is a key pressed during a drag. Escape drops the selection and
// copies nothing, and quit still quits. Every other key is dropped, because it
// would change a screen the selection stands over.
func (m Model) selectingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case m.keys.Match(config.ShellContext, config.ShellActionEscape, msg):
		m.press, m.sel = nil, textSelection{}
	case m.keys.Match(config.ShellContext, config.ShellActionQuit, msg):
		return m, tea.Quit
	}
	return m, nil
}

// copyToClipboard puts text on the system clipboard with OSC 52, which reaches
// the clipboard of the machine the terminal runs on, over ssh too. It is one
// write of a sequence that draws nothing, so it cannot corrupt a frame the
// renderer is writing.
func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		_, _ = os.Stdout.WriteString(ansi.SetSystemClipboard(text))
		return nil
	}
}

func abs(n int) int {
	return max(n, -n)
}
