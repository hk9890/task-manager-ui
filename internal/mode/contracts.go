package mode

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// ID identifies a top-level workflow hosted by the root shell.
type ID string

const (
	Board  ID = "board"
	Docs   ID = "docs"
	Search ID = "search"
	Detail ID = "detail"
)

// BrowseModes lists the browse tabs in header order. Detail is not a tab: it is
// a drill-in reached from a browse mode and left with Escape.
var BrowseModes = []ID{Board, Docs, Search}

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

// Browse is the contract every browse tab satisfies. Board, Docs and Search
// already had this exact method set; without an interface to hold them the
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

// SelectionChangedMsg is emitted by board/search modes whenever the selected
// issue changes so the shell can update detail presentation state.
type SelectionChangedMsg struct {
	Mode      ID
	Selection *Selection
}

// ActionRequestMsg is emitted by browse modes for shell-owned actions.
type ActionRequestMsg struct {
	Mode   ID
	Action Action
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
