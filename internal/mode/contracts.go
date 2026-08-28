package mode

import (
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
)
