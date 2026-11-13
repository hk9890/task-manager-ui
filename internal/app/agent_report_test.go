package app

// How the shell names an agent and reports what it did: one name per agent
// in every toast, and the report of an asked agent read to its end.

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/mode"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// namedOnTab9 is an agent whose harness names it: its title is the name.
func namedOnTab9() agent.Agent {
	return agent.Agent{Ref: "demo:home:9", Session: "s-9", Label: "Tab 9", Name: "task-manager-ui-9", Status: agent.Idle}
}

// The start toast of an intent names the agent by its title, as its row and
// the close toast do.
func TestTheStartToastNamesTheAgentByItsTitle(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Started = namedOnTab9()

	m = pressKey(t, m, "alt+i")
	m = pressKey(t, m, "enter")
	if view := m.toast.View(); !strings.Contains(view, "Started task-manager-ui-9 on tm-1") {
		t.Errorf("toast = %q, want the start reported by the agent's name", view)
	}
}

// The report of an asked agent names it by its title too.
func TestTheAskReportNamesTheAgentByItsTitle(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Started = namedOnTab9()
	client.Said = `{"status":"done","summary":"Shortened the title."}`
	m = askOnTitle(t, m, "make the title shorter")
	if view := m.toast.View(); !strings.Contains(view, "task-manager-ui-9 on tm-1 is done: Shortened the title.") {
		t.Errorf("toast = %q, want the report by the agent's name", view)
	}
}

// The Detail legend names the agent dialog key as the agent button does.
func TestTheDetailLegendNamesTheAgentKeyAsTheButtonDoes(t *testing.T) {
	m, _, _, _ := agentShell(t)
	footer := testui.Downsample(m.renderFooter())
	if !strings.Contains(footer, "alt+i agent ") || strings.Contains(footer, "alt+i agents") {
		t.Errorf("legend = %q, want alt+i agent", footer)
	}
}

// answered hands the shell the reply of the asked agent on tm-1 and reports
// whether the toast of its report got a dismiss timer.
func answered(t *testing.T, m Model, reply string) (Model, bool) {
	t.Helper()
	scheduled := false
	m.scheduleToastDismiss = func(time.Duration, int) tea.Cmd {
		scheduled = true
		return nil
	}
	m = updateOnly(m, agentAnsweredMsg{issueID: "tm-1", agent: namedOnTab9(), stoppedOn: agent.Idle, reply: reply})
	return m, scheduled
}

// A summary too long for one line wraps over the lines of the report and
// closes as every toast does.
func TestTheAskReportWrapsItsSummary(t *testing.T) {
	m, _, _, _ := agentShell(t)
	summary := "Shortened the title of the issue so it fits the board column, and moved the long " +
		"explanation into the description where it belongs, closing tail."
	m, scheduled := answered(t, m, `{"status":"done","summary":"`+summary+`"}`)
	if view := plainShell(m); !strings.Contains(view, "closing tail.") {
		t.Errorf("the report cuts its summary:\n%s", view)
	}
	if !scheduled {
		t.Error("a report that fits got no dismiss timer")
	}
}

// A summary that does not fit the lines of the report is cut, and the toast
// stays until Escape, which does nothing else.
func TestAnAskReportThatIsCutStaysUntilEscape(t *testing.T) {
	m, _, _, _ := agentShell(t)
	summary := strings.Repeat("word ", 80) + "tail."
	m, scheduled := answered(t, m, `{"status":"failed","summary":"`+summary+`"}`)
	if scheduled {
		t.Error("a report that is cut got a dismiss timer")
	}
	view := plainShell(m)
	if !strings.Contains(view, "failed: word word") || !strings.Contains(view, "…") || strings.Contains(view, "tail.") {
		t.Errorf("the cut report:\n%s", view)
	}

	m = pressKey(t, m, "esc")
	if m.toast.Visible() || m.active != mode.Detail {
		t.Errorf("esc: toast visible %v, on %q; want the toast hidden and Detail kept", m.toast.Visible(), m.active)
	}
}
