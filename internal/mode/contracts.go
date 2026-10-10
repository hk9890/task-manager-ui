package mode

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// ID identifies a top-level workflow hosted by the root shell.
type ID string

const (
	Board ID = "board"
	Docs  ID = "docs"
	// Search is the store search. It satisfies Browse and holds a selection,
	// but it is not a tab: it is absent from BrowseModes and is opened by a
	// shell action instead.
	Search ID = "search"
	Detail ID = "detail"
	// StorePicker lists the central task-manager stores on this machine. It is
	// neither a tab nor a drill-in: it sits above every browse surface and
	// renders instead of the shell.
	StorePicker ID = "store_picker"
)

// BrowseModes lists the browse tabs in header order. Neither Detail nor
// StorePicker is a tab: Detail is a drill-in reached from a browse mode and
// left with Escape, and StorePicker is a full-screen surface above all of them.
var BrowseModes = []ID{Board, Docs}

// IsBrowse reports whether id is one of the browse tabs. The shell keeps
// lastBrowse pointing at one of these, so selection lookups always resolve.
func IsBrowse(id ID) bool {
	for _, browse := range BrowseModes {
		if browse == id {
			return true
		}
	}
	return false
}

// Browse is the contract every browse tab satisfies, and the store search with
// them. Board and Docs already had this exact method set; without an interface to hold them the
// shell hand-wrote the same dispatch once per tab at eight sites, and a missed
// site was silent behavioural drift rather than a build error.
//
// A new browse surface is an entry in BrowseModes, a registration in the
// shell's map, and one tab(...) call in the header (DESIGN-GUIDE.md).
type Browse interface {
	// Init starts the tab's first load. The shell calls it lazily, on first
	// entry, rather than at startup.
	Init() tea.Cmd

	// Update handles one message routed to this tab.
	Update(msg tea.Msg) tea.Cmd

	// View renders the tab. skeletonPhase drives the cold-start pulse.
	View(skeletonPhase int) string

	// SetSize gives the tab its workspace dimensions.
	SetSize(width, height int)

	// IsLoading reports whether work is in flight, which drives the header
	// spinner and suppresses a duplicate auto-refresh.
	IsLoading() bool

	// AutoRefresh returns the periodic reload command, or nil when the tab has
	// nothing to refresh.
	AutoRefresh() tea.Cmd

	// Reload is what the tab's reload key runs, which the shell's reload
	// button runs too. It returns nil while a load is already in flight.
	Reload() tea.Cmd

	// ClearQuery empties the tab's filter query. The shell calls it on Escape
	// before its own handling: cleared is false when the query was already
	// empty, and Escape then does what it does without a query.
	ClearQuery() (cleared bool, cmd tea.Cmd)

	// TakesKey reports whether the tab takes this key for itself before any
	// binding. The shell asks the tab on screen, and runs no action on a key
	// it takes.
	TakesKey(msg tea.KeyMsg) bool
}

// RefreshMode distinguishes the two reasons a browse tab reloads. It lives here
// rather than in one tab's package because every browse tab makes the same
// distinction, and a bare boolean at the call site — startReload(true) — says
// nothing about which of the two it means.
type RefreshMode int

const (
	// RefreshReload is a full reset of tab state: focus, selection, scroll and
	// content. It is the cold-start load and the operator's reload key.
	RefreshReload RefreshMode = iota

	// RefreshAuto is a background refresh that preserves the selection anchor
	// instead of resetting the tab.
	RefreshAuto
)

// Selection identifies the issue currently selected by a browse mode.
type Selection struct {
	Issue domain.IssueSummary
}

// SelectionChangedMsg is emitted by browse modes whenever the selected
// issue changes so the shell can update detail presentation state.
type SelectionChangedMsg struct {
	Mode      ID
	Selection *Selection
}

// ActionRequestMsg is emitted by browse modes for shell-owned actions.
type ActionRequestMsg struct {
	Mode   ID
	Action Action
	// Selection is the row an ActionOpenDetail opens: the selection the mode
	// held when it asked. The shell adopts it before it opens the detail, so
	// the request does not depend on a SelectionChangedMsg arriving first. The
	// dialog actions leave it nil.
	Selection *Selection
}

// RequestActionCmd returns the Cmd that asks the shell for a shell-owned action
// on behalf of the mode with this id.
//
// It and RequestOpenDetailCmd are the constructors of the whole contract, so
// no mode builds the message inline.
func RequestActionCmd(id ID, action Action) tea.Cmd {
	return func() tea.Msg {
		return ActionRequestMsg{Mode: id, Action: action}
	}
}

// RequestOpenDetailCmd returns the Cmd that asks the shell to open the detail
// of selection, the row the mode with this id holds selected.
func RequestOpenDetailCmd(id ID, selection *Selection) tea.Cmd {
	return func() tea.Msg {
		return ActionRequestMsg{Mode: id, Action: ActionOpenDetail, Selection: selection}
	}
}

// Action identifies a shell-level action entry point.
type Action string

const (
	ActionOpenDetail Action = "open_detail"

	// ActionOpenStatusDialog and ActionOpenPriorityDialog are the metadata
	// quick-edit entry points. The shell owns the dialogs, so a mode asks for
	// one through this contract rather than parking a flag for the shell to
	// poll after every key press.
	ActionOpenStatusDialog   Action = "open_status_dialog"
	ActionOpenPriorityDialog Action = "open_priority_dialog"
)

// MouseKind is what the pointer did.
type MouseKind int

const (
	// MouseMove is the pointer arriving on a cell, with no button involved.
	MouseMove MouseKind = iota
	// MouseLeave is the pointer leaving the surface: it carries no cell.
	MouseLeave
	// MouseClick is a press of the left button.
	MouseClick
	MouseWheelUp
	MouseWheelDown
)

// MouseMsg is one pointer event for a surface, in that surface's own
// coordinates: the shell subtracts its chrome, so (0, 0) is the first cell the
// surface draws. Only the surface on screen receives a cell; every other one
// receives a MouseLeave.
type MouseMsg struct {
	Kind MouseKind
	X, Y int
	// At is when the event arrived, which is what tells a double click from
	// two single ones.
	At time.Time
}

// Pointer is the cell the mouse is over, in a surface's own coordinates. A
// surface keeps the one its last MouseMsg left and derives its hover from it
// on every draw, so a row that scrolls or reloads under a still pointer is the
// one that lights up.
type Pointer struct {
	X, Y int
}

// Pointer is where msg leaves the pointer: on its cell, or nil once it has
// left the surface.
func (msg MouseMsg) Pointer() *Pointer {
	if msg.Kind == MouseLeave {
		return nil
	}
	return &Pointer{X: msg.X, Y: msg.Y}
}

// doubleClickWindow is how close two clicks on one target are to count as a
// double click. The desktop's own is between 400 and 500 ms.
const doubleClickWindow = 400 * time.Millisecond

// ClickTracker tells a double click from two single ones. A click selects and
// a second click on the same target opens it, so every surface with rows keeps
// one.
type ClickTracker struct {
	target string
	x, y   int
	at     time.Time
}

// Double records a click and reports whether it is the second of a double
// click. target names the row under the pointer, "" for none, and selected the
// row the surface holds selected as the click arrives.
//
// The second click counts when it lands on the row the first one selected, or
// on the same cell: selecting a row can scroll its list — a row drawn with
// only its first line is pulled into the window — and the operator
// double-clicking in place means the row they just selected, not the one that
// slid under the pointer. So on true the caller opens its current selection
// and selects nothing new. That is the row the first click selected only while
// nothing else moved the selection: a wheel notch, a key or a reload between
// the two clicks makes the second one a single click again. A double click is
// consumed, and a third click starts over.
func (t *ClickTracker) Double(target, selected string, msg MouseMsg) bool {
	last := *t
	*t = ClickTracker{target: target, x: msg.X, y: msg.Y, at: msg.At}
	sameCell := last.x == msg.X && last.y == msg.Y
	if last.target == "" || last.target != selected || (target != last.target && !sameCell) || msg.At.Sub(last.at) >= doubleClickWindow {
		return false
	}
	*t = ClickTracker{}
	return true
}
