package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/mode"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
)

func TestAssertionHelpersCoverStartupErrorsSearchAndActions(t *testing.T) {
	t.Parallel()

	t.Run("startup sanity and no obvious errors", func(t *testing.T) {
		output := "Default\nNot Ready\nReady\nIn Progress\n│││││"
		AssertStartupBoardLayoutSanity(t, output)
		AssertNoObviousRuntimeErrorPanels(t, output)
	})

	t.Run("latest search query text", func(t *testing.T) {
		calls := []fakes.GatewayCall{{
			Method: fakes.MethodSearchIssues,
			Input:  fakes.SearchIssuesCall{Query: domain.SearchIssuesQuery{Text: "jkhlr"}},
		}}
		AssertLatestSearchQueryText(t, calls, "jkhlr")
	})

	t.Run("action request", func(t *testing.T) {
		msg := tea.Msg(mode.ActionRequestMsg{Mode: mode.Search, Action: mode.ActionOpenDetail})
		AssertActionRequest(t, msg, mode.Search, mode.ActionOpenDetail)
	})
}
