package app

// Ask agent: the selection menu asks an agent in the background about the
// selected text, waits for its turn to end, reloads the issue and reports
// the status line of its reply.

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

// askOnTitle selects the title of tm-1 in Detail, asks an agent about it with
// instruction, and returns once the fake host has answered.
func askOnTitle(t *testing.T, m Model, instruction string) Model {
	t.Helper()
	m, cmd := askCmdOnTitle(t, m, instruction)
	return applyMessages(t, m, []tea.Msg{cmd()})
}

// askCmdOnTitle is askOnTitle up to the Ask button: it returns the command
// Enter on Ask produced, not run yet.
func askCmdOnTitle(t *testing.T, m Model, instruction string) (Model, tea.Cmd) {
	t.Helper()
	m.copyText = func(string) tea.Cmd { return nil }
	x, y := testui.FindCell(t, testui.View(m), "Ready first")
	endX := x + len("Ready first") - 1
	m = send(t, m, testui.Click(x, y, tea.MouseLeft))
	m = send(t, m, testui.Drag(endX, y))
	m = send(t, m, testui.Release(endX, y))
	if m.selMenu == nil {
		t.Fatal("the release of a selection opened no menu")
	}
	testui.AssertContainsAll(t, plainShell(m), "Ask an agent about it on tm-1")
	m = pressKey(t, m, "down")
	// Stepped by hand from here: the question's input schedules a repeating
	// cursor tick, so draining its commands never returns (docs/TESTING.md).
	m = updateOnly(m, testui.Press("enter"))
	if !m.showActionModal || m.agentAsk == nil {
		t.Fatal("Ask an agent opened no question")
	}
	testui.AssertContainsAll(t, plainShell(m), "Ask an agent", "About the selection on tm-1")
	m = updateOnly(m, testui.Runes(instruction))
	m = updateOnly(m, testui.Press("enter")) // to the Ask button
	next, cmd := m.Update(testui.Press("enter"))
	if cmd == nil {
		t.Fatal("Enter on Ask produced no command")
	}
	return next.(Model), cmd
}

// updateOnly hands m one message and drops the commands it returns.
func updateOnly(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func askedShell(t *testing.T, said string) (Model, *fakes.FakeAgentHost) {
	t.Helper()
	m, client, _, _ := agentShell(t)
	client.Started = startedOnTab9()
	client.Said = said
	return askOnTitle(t, m, "make the title shorter"), client
}

// The agent starts without the focus, prompted with the issue, the selection,
// the instruction and the status line to end with. It is linked to the issue,
// waited on and read, and stays open. Its summary is reported.
func TestAskAnAgentStartsItInTheBackgroundAndReportsItsSummary(t *testing.T) {
	m, client := askedShell(t, "I shortened it.\n"+`{"status":"done","summary":"Shortened the title."}`)

	specs := startCalls(client)
	if len(specs) != 1 || specs[0].Root != m.services.ProjectRoot {
		t.Fatalf("Start calls = %+v, want one without the focus in the project", specs)
	}
	for _, part := range []string{"tm-1", `"Ready first"`, "make the title shorter", `{"status":"done"|"failed"|"question"`} {
		if !strings.Contains(specs[0].Prompt, part) {
			t.Errorf("prompt %q does not hold %q", specs[0].Prompt, part)
		}
	}
	if strings.Contains(specs[0].Prompt, "\n") {
		t.Errorf("prompt %q is more than one line", specs[0].Prompt)
	}
	want := []fakes.AgentHostCall{{Method: "Wait", Arg: "demo:home:9"}, {Method: "Read", Arg: "demo:home:9"}}
	if got := hostCalls(client, "Wait", "Read", "Focus", "Close"); !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %+v, want %+v", got, want)
	}
	if labels := labelsOf(t, m, "tm-1"); !reflect.DeepEqual(labels, []string{"agent-session:s-9"}) {
		t.Fatalf("labels of tm-1 = %v, want [agent-session:s-9]", labels)
	}
	if view := m.toast.View(); !strings.Contains(view, "Tab 9 on tm-1 is done: Shortened the title.") {
		t.Fatalf("toast = %q, want the summary", view)
	}
}

func TestAnAgentThatFailedIsReportedAsAnError(t *testing.T) {
	m, _ := askedShell(t, `{"status":"failed","summary":"No such field."}`)
	if view := m.toast.View(); !strings.Contains(view, "Tab 9 on tm-1 failed: No such field.") {
		t.Fatalf("toast = %q, want the failure", view)
	}
}

// A question offers to bring the agent to the front: yes focuses it.
func TestAnAgentThatAsksIsOfferedToTheFront(t *testing.T) {
	m, client := askedShell(t, "```json\n"+`{"status":"question","summary":"Which title?"}`+"\n```")
	if !m.showActionModal || m.agentFocusOffer == nil {
		t.Fatal("a question offered no focus")
	}
	testui.AssertContainsAll(t, plainShell(m), "The agent asks", "Which title?")
	m = pressKey(t, m, "y")
	if m.showActionModal {
		t.Fatal("yes left the offer open")
	}
	if got := hostCalls(client, "Focus"); len(got) != 1 || got[0].Arg != "demo:home:9" {
		t.Fatalf("Focus calls = %+v, want one on Tab 9", got)
	}
}

// An agent whose turn stopped to wait for the operator is offered to the
// front and not read: it has not finished.
func TestAnAgentThatWaitsForYouIsOfferedToTheFront(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Started = startedOnTab9()
	client.StoppedOn = agent.NeedsYou
	client.Said = `{"status":"done","summary":"stale"}`
	m = askOnTitle(t, m, "make the title shorter")

	if !m.showActionModal || m.agentFocusOffer == nil {
		t.Fatalf("an agent that waits for you offered no focus; toast %q", m.toast.View())
	}
	testui.AssertContainsAll(t, plainShell(m), "The agent asks", "Tab 9 on tm-1 waits for you")
	if got := hostCalls(client, "Read"); len(got) != 0 {
		t.Fatalf("Read calls = %+v, want none", got)
	}
}

// While the operator types a query, the offer would take the next key: a
// warning toast points to the agent dialog instead.
func TestAQuestionWhileAQueryIsTypedIsAToast(t *testing.T) {
	m, _, _, _ := agentShell(t)
	m = pressKey(t, m, "esc")
	m = updateOnly(m, testui.Runes("rea"))
	if m.active != mode.Board || !m.board.HasQuery() {
		t.Fatalf("fixture is on %q with query %v, want Board with a query", m.active, m.board.HasQuery())
	}
	m = send(t, m, agentAnsweredMsg{issueID: "tm-1", agent: startedOnTab9(), stoppedOn: agent.Idle, reply: `{"status":"question","summary":"Which?"}`})

	if m.showActionModal {
		t.Fatal("a question while a query is typed opened the offer")
	}
	if view := m.toast.View(); !strings.Contains(view, "answer it from the agent dialog") {
		t.Fatalf("toast = %q, want the pointer to the agent dialog", view)
	}
}

func TestAReplyWithoutAStatusLineIsReportedAsFinished(t *testing.T) {
	m, _ := askedShell(t, `Done. {"status": maybe}`)
	if view := m.toast.View(); !strings.Contains(view, "Tab 9 on tm-1 finished") {
		t.Fatalf("toast = %q, want the agent reported as finished", view)
	}
}

// writingHost is a host whose agent renames tm-1 during its turn.
type writingHost struct {
	*fakes.FakeAgentHost
	repo interface {
		UpdateIssue(context.Context, string, domain.UpdateIssueInput) error
	}
}

func (h writingHost) Wait(ctx context.Context, a agent.Agent) (agent.Status, error) {
	title := "Renamed by the agent"
	if err := h.repo.UpdateIssue(ctx, "tm-1", domain.UpdateIssueInput{Title: &title}); err != nil {
		return "", err
	}
	return h.FakeAgentHost.Wait(ctx, a)
}

// The end of the turn reloads Detail at once, so it shows what the agent
// wrote.
func TestTheEndOfTheTurnReloadsTheIssue(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Started = startedOnTab9()
	m.services.Agents = writingHost{FakeAgentHost: client, repo: m.services.Repo}
	m = askOnTitle(t, m, "rename it")
	testui.AssertContainsAll(t, plainShell(m), "Renamed by the agent")
}

// An asked agent that opened but did not take the prompt is linked, and not
// waited on: it has no turn.
func TestAnAskedAgentThatTookNoPromptIsLinkedAndNotWaitedOn(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Started = startedOnTab9()
	client.StartErr = errors.New("the prompt was refused")
	m = askOnTitle(t, m, "make the title shorter")

	if labels := labelsOf(t, m, "tm-1"); !reflect.DeepEqual(labels, []string{"agent-session:s-9"}) {
		t.Fatalf("labels of tm-1 = %v, want [agent-session:s-9]", labels)
	}
	if got := hostCalls(client, "Wait", "Read"); len(got) != 0 {
		t.Fatalf("calls = %+v, want no wait and no read", got)
	}
	if view := m.toast.View(); !strings.Contains(view, "Started Tab 9 on tm-1, but could not ask it: the prompt was refused") {
		t.Fatalf("toast = %q, want the partial failure", view)
	}
}

// On a store that is not watched the link of an asked agent reloads the
// issue at once, so the agent shows on it before its turn ends.
func TestTheLinkOfAnAskedAgentReloadsTheIssue(t *testing.T) {
	m, _, _, _ := agentShell(t)
	if m.storeWatched() {
		t.Fatal("fixture store is watched")
	}
	m.handleAgentAskStarted(agentAskStartedMsg{epoch: m.storeEpoch, issueID: "tm-1", agent: startedOnTab9()})
	if !m.detail.IsLoading() {
		t.Fatal("the link of an asked agent did not reload the issue")
	}
}

func TestParseAskReportReadsTheLastStatusLine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		reply string
		want  askReport
		ok    bool
	}{
		{reply: `{"status":"done","summary":"ok"}`, want: askReport{Status: "done", Summary: "ok"}, ok: true},
		{reply: "`{\"status\":\"failed\",\"summary\":\"no\"}`", want: askReport{Status: "failed", Summary: "no"}, ok: true},
		{reply: `{"status":"done","summary":"first"}` + "\n" + `{"status":"question","summary":"last"}` + "\nbye", want: askReport{Status: "question", Summary: "last"}, ok: true},
		{reply: `{"status":"maybe","summary":"x"}`},
		{reply: "no status at all"},
		{reply: ""},
	}
	for _, tc := range cases {
		got, ok := parseAskReport(tc.reply)
		if ok != tc.ok || got != tc.want {
			t.Errorf("parseAskReport(%q) = %+v, %v; want %+v, %v", tc.reply, got, ok, tc.want, tc.ok)
		}
	}
}

// On a closed issue the store refuses the link label: Ask is off and says
// why.
func TestAskIsOffOnAClosedIssue(t *testing.T) {
	m, _, _, _ := agentShell(t)
	m.detail.Detail.Summary.Status = domain.StatusClosed
	m.copyText = func(string) tea.Cmd { return nil }
	x, y := testui.FindCell(t, testui.View(m), "Ready first")
	m = send(t, m, testui.Click(x, y, tea.MouseLeft))
	m = send(t, m, testui.Drag(x+4, y))
	m = send(t, m, testui.Release(x+4, y))

	testui.AssertContainsAll(t, plainShell(m), "Ask an agent: off, tm-1 is closed")
	if ask := m.selMenu.entries[1].entry; !ask.Disabled {
		t.Errorf("the ask entry %q is on for a closed issue", ask.Label)
	}
}

// Outside Detail the issue of the selection says whether it is closed: the
// Agents tab lists an agent linked to a closed issue.
func TestAskIsOffOnAClosedIssueOfTheAgentsTab(t *testing.T) {
	m, _ := agentsTab(t)
	if err := m.services.Repo.CloseIssue(m.ctx, "tm-1", domain.CloseIssueInput{Reason: "done"}); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	m = pressKey(t, m, "alt+r")
	if sel := m.currentSelection(); sel == nil || sel.Issue.Status != domain.StatusClosed {
		t.Fatalf("fixture: the selection is %+v, want closed tm-1", sel)
	}
	m.copyText = func(string) tea.Cmd { return nil }
	x, y := testui.FindCell(t, testui.View(m), "Ready first")
	m = send(t, m, testui.Click(x, y, tea.MouseLeft))
	m = send(t, m, testui.Drag(x+4, y))
	m = send(t, m, testui.Release(x+4, y))

	testui.AssertContainsAll(t, plainShell(m), "Ask an agent: off, tm-1 is closed")
	if ask := m.selMenu.entries[1].entry; !ask.Disabled {
		t.Errorf("the ask entry %q is on for a closed issue", ask.Label)
	}
}

// A start that fails before the agent has a session links nothing and waits
// on nothing.
func TestAnAskWhoseStartFailsIsReportedAndNotWaitedOn(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.StartErr = errors.New("revier is not running")
	m = askOnTitle(t, m, "make the title shorter")

	if view := m.toast.View(); !strings.Contains(view, "Could not ask an agent on tm-1: revier is not running") {
		t.Errorf("toast = %q, want the failed start", view)
	}
	if got := hostCalls(client, "Wait", "Read"); len(got) != 0 {
		t.Errorf("calls = %+v, want no wait and no read", got)
	}
	if labels := labelsOf(t, m, "tm-1"); len(labels) != 0 {
		t.Errorf("labels of tm-1 = %v, want none", labels)
	}
	if m.workInFlight() {
		t.Error("the spinner still turns after the start ended")
	}
}

// A wait that fails is reported: the agent did not report back.
func TestAnAskedAgentWhoseWaitFailsIsReported(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Started = startedOnTab9()
	client.WaitErr = errors.New("the turn timed out")
	m = askOnTitle(t, m, "make the title shorter")

	if view := m.toast.View(); !strings.Contains(view, "Tab 9 on tm-1 did not report back: the turn timed out") {
		t.Errorf("toast = %q, want the failed wait", view)
	}
	if got := hostCalls(client, "Read"); len(got) != 0 {
		t.Errorf("Read calls = %+v, want none", got)
	}
}

// askStarted runs an ask on tm-1 up to the start's result and hands the
// shell that result: the returned command, not run, is the wait.
func askStarted(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	m, submit := askCmdOnTitle(t, m, "make the title shorter")
	next, start := m.Update(submit())
	m = next.(Model)
	if !m.workInFlight() {
		t.Error("the spinner does not turn while the start is in flight")
	}
	for _, msg := range runBatch(start) {
		if started, ok := msg.(agentAskStartedMsg); ok {
			next, wait := m.Update(started)
			return next.(Model), wait
		}
	}
	t.Fatal("the ask started no agent")
	return m, nil
}

// An agent that started but was not linked is reported, and still waited on:
// it works on the issue.
func TestAnAskedAgentThatIsNotLinkedIsReportedAndWaitedOn(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Started = startedOnTab9()
	failing := fakes.NewErrorInjecting(m.services.Repo)
	failing.SetError(fakes.MethodUpdateIssue, errors.New("store locked"))
	m.services.Repo = failing

	m, wait := askStarted(t, m)
	if view := m.toast.View(); !strings.Contains(view, "Tab 9 works on tm-1, but could not be linked to it: store locked") {
		t.Errorf("toast = %q, want the failed link", view)
	}
	if m.workInFlight() {
		t.Error("the spinner still turns after the start ended")
	}
	applyMessages(t, m, runBatch(wait))
	if got := hostCalls(client, "Wait"); len(got) != 1 {
		t.Errorf("Wait calls = %+v, want one", got)
	}
}

// An agent that took no prompt and was not linked is reported with both
// errors, and not waited on.
func TestAnAskedAgentThatTookNoPromptAndIsNotLinkedReportsBoth(t *testing.T) {
	m, client, _, _ := agentShell(t)
	client.Started = startedOnTab9()
	client.StartErr = errors.New("the prompt was refused")
	failing := fakes.NewErrorInjecting(m.services.Repo)
	failing.SetError(fakes.MethodUpdateIssue, errors.New("store locked"))
	m.services.Repo = failing

	m, wait := askStarted(t, m)
	want := "Started Tab 9 on tm-1, but could not ask it: the prompt was refused; and could not link it: store locked"
	if view := m.toast.View(); !strings.Contains(view, want) {
		t.Errorf("toast = %q, want %q", view, want)
	}
	applyMessages(t, m, runBatch(wait))
	if got := hostCalls(client, "Wait"); len(got) != 0 {
		t.Errorf("Wait calls = %+v, want none", got)
	}
}

// A start that returns after a store switch and could not be linked says so,
// and names the store it began in.
func TestAnAskStartedInTheStoreLeftThatIsNotLinkedIsReported(t *testing.T) {
	m, client, _, _ := agentShell(t)
	m = send(t, m, agentAskStartedMsg{epoch: m.storeEpoch - 1, issueID: "tm-1", agent: startedOnTab9(), linkErr: errors.New("store locked")})

	want := "Tab 9 works on tm-1 in the store you left, but could not be linked to it: store locked"
	if view := m.toast.View(); !strings.Contains(view, want) {
		t.Errorf("toast = %q, want %q", view, want)
	}
	if got := hostCalls(client, "Wait"); len(got) != 0 {
		t.Errorf("Wait calls = %+v, want none", got)
	}
}
