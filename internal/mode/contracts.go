package mode

import (
	"github.com/hk9890/beads-workbench/internal/domain"
)

// ID identifies a top-level workflow hosted by the root shell.
type ID string

const (
	Board  ID = "board"
	Search ID = "search"
	Detail ID = "detail"
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
	ActionLaunch     Action = "launch"
)
