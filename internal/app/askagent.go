package app

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/agent"
	"github.com/hk9890/task-manager-ui/internal/displaytext"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/ui/modal"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

// agentAsk is what the ask question of the action modal is about: the issue
// the selection menu named and the selected text, without the chrome.
type agentAsk struct {
	issue domain.IssueSummary
	text  string
}

// askInstructionKey is the key of the one input of the ask question.
const askInstructionKey = "instruction"

// The statuses the asked agent reports on its last line.
const (
	askDone     = "done"
	askFailed   = "failed"
	askQuestion = "question"
)

// askReport is the JSON line the asked agent ends its reply with.
type askReport struct {
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

// openAgentAsk opens the action modal with one line for the instruction to
// the agent about text on issue.
func (m *Model) openAgentAsk(issue domain.IssueSummary, text string) tea.Cmd {
	m.agentAsk = &agentAsk{issue: issue, text: text}
	return m.openActionModal(modal.Config{
		Title:       "Ask an agent",
		Message:     "About the selection on " + displaytext.OneLine(issue.ID) + ". It works in the background.",
		Inputs:      []modal.InputConfig{{Key: askInstructionKey, Label: "Instruction", Placeholder: "what to change"}},
		ConfirmText: "Ask",
		MinWidth:    72,
	})
}

// submitAgentAsk closes the ask question and asks the agent. An empty
// instruction leaves the question open.
func (m *Model) submitAgentAsk(values map[string]string) tea.Cmd {
	instruction := strings.Join(strings.Fields(values[askInstructionKey]), " ")
	if instruction == "" {
		return nil
	}
	ask := *m.agentAsk
	m.showActionModal = false
	m.agentAsk = nil
	return m.askAgent(ask, instruction)
}

// askAgent starts an agent in the background on the issue, prompted with the
// selection and the instruction, and links it to the issue with the label of
// its session, as startAgent does.
func (m *Model) askAgent(ask agentAsk, instruction string) tea.Cmd {
	if off := m.agentsOff(); off != "" {
		return m.showToast(off, toaster.StyleWarn)
	}
	host, repo, root := m.services.Agents, m.services.Repo, m.services.ProjectRoot
	issueID := ask.issue.ID
	spec := agent.Spec{Root: root, Prompt: askPrompt(issueID, ask.text, instruction)}
	asking := m.showToast("Asking an agent on "+displaytext.OneLine(issueID)+"…", toaster.StyleInfo)
	epoch := m.storeEpoch
	m.agentStarts++
	start := func() tea.Msg {
		return agentAskStartedMsg(startAndLink(host, repo, epoch, issueID, spec))
	}
	return batchCmds(asking, start)
}

// askPrompt is the one line the asked agent is prompted with: the host
// submits a prompt as one line, so the selection is put on one.
func askPrompt(issueID, text, instruction string) string {
	selection := strings.Join(strings.Fields(strings.Join(quotable(text), " ")), " ")
	return fmt.Sprintf("In taskmgr issue %s the operator selected this text: %q. Their instruction: %q. "+
		"Read the issue with taskmgr, apply the change to the issue with taskmgr, and change nothing else. "+
		`End your reply with exactly one line of JSON: {"status":"done"|"failed"|"question","summary":"<one sentence>"}.`,
		issueID, selection, instruction)
}

// handleAgentAskStarted reports a start that failed, or reloads the issue the
// label was written to and waits in the background for the turn of the agent
// that started. An agent that opened but did not take the prompt is linked
// and not waited on: it has no turn. After a store switch the start is
// reported and not waited on: the wait belongs to the store it began on.
func (m *Model) handleAgentAskStarted(msg agentAskStartedMsg) tea.Cmd {
	m.agentStarts = max(m.agentStarts-1, 0)
	issueID := displaytext.OneLine(msg.issueID)
	if msg.err != nil && msg.agent.Session == "" {
		return m.showToast(fmt.Sprintf("Could not ask an agent on %s: %v", issueID, msg.err), toaster.StyleError)
	}
	label := displaytext.OneLine(msg.agent.Title())
	var reload tea.Cmd
	if msg.linkErr == nil {
		reload = m.reloadAfterStart(agentStart(msg))
	}
	switch {
	case msg.err == nil && msg.epoch != m.storeEpoch && msg.linkErr != nil:
		return m.showToast(fmt.Sprintf("%s works on %s in the store you left, but could not be linked to it: %v", label, issueID, msg.linkErr), toaster.StyleError)
	case msg.err == nil && msg.epoch != m.storeEpoch:
		return m.showToast(fmt.Sprintf("%s works on %s in the store you left; its answer is not reported", label, issueID), toaster.StyleInfo)
	case msg.err != nil && msg.linkErr != nil:
		return m.showToast(fmt.Sprintf("Started %s on %s, but could not ask it: %v; and could not link it: %v", label, issueID, msg.err, msg.linkErr), toaster.StyleError)
	case msg.err != nil:
		return batchCmds(reload, m.showToast(fmt.Sprintf("Started %s on %s, but could not ask it: %v", label, issueID, msg.err), toaster.StyleError))
	case msg.linkErr != nil:
		report := m.showToast(fmt.Sprintf("%s works on %s, but could not be linked to it: %v", label, issueID, msg.linkErr), toaster.StyleError)
		return batchCmds(report, m.waitForAnswer(msg.issueID, msg.agent))
	default:
		report := m.showToast(fmt.Sprintf("%s works on %s in the background", label, issueID), toaster.StyleInfo)
		return batchCmds(reload, report, m.waitForAnswer(msg.issueID, msg.agent))
	}
}

// waitForAnswer waits for the turn of the asked agent to end and reads what
// it said. It runs on the store's context and is scoped: a store switch ends
// the wait and drops its result. The agent stays open either way.
func (m *Model) waitForAnswer(issueID string, a agent.Agent) tea.Cmd {
	ctx, host := m.ctx, m.services.Agents
	return m.scoped(func() tea.Msg {
		stoppedOn, err := host.Wait(ctx, a)
		if err != nil || stoppedOn == agent.NeedsYou {
			return agentAnsweredMsg{issueID: issueID, agent: a, stoppedOn: stoppedOn, err: err}
		}
		reply, err := host.Read(ctx, a)
		return agentAnsweredMsg{issueID: issueID, agent: a, stoppedOn: stoppedOn, reply: reply, err: err}
	})
}

// handleAgentAnswered polls the agents and, on a store that is not watched,
// reloads what the agent may have written at once: on a watched one the
// watch reloads after the agent's writes. Then it reports the status line of
// its reply. An agent that stopped to wait for the operator, and a question,
// offer to bring the agent to the front. A reply without a status line is
// reported as finished: the reload shows what it did.
func (m *Model) handleAgentAnswered(msg agentAnsweredMsg) tea.Cmd {
	reload := []tea.Cmd{m.pollAgentsCmd(), m.reloadAfterLink(msg.issueID)}

	label, issueID := displaytext.OneLine(msg.agent.Title()), displaytext.OneLine(msg.issueID)
	var report tea.Cmd
	if msg.err != nil {
		report = m.showToast(fmt.Sprintf("%s on %s did not report back: %v", label, issueID, msg.err), toaster.StyleError)
		return batchCmds(append(reload, report)...)
	}
	if msg.stoppedOn == agent.NeedsYou {
		report = m.offerAgentFocus(msg.agent, fmt.Sprintf("%s on %s waits for you", label, issueID))
		return batchCmds(append(reload, report)...)
	}
	parsed, ok := parseAskReport(msg.reply)
	switch {
	case !ok:
		report = m.showToast(fmt.Sprintf("%s on %s finished", label, issueID), toaster.StyleInfo)
	case parsed.Status == askDone:
		report = m.showReport(withSummary(fmt.Sprintf("%s on %s is done", label, issueID), parsed.Summary), toaster.StyleSuccess)
	case parsed.Status == askFailed:
		report = m.showReport(withSummary(fmt.Sprintf("%s on %s failed", label, issueID), parsed.Summary), toaster.StyleError)
	default:
		report = m.offerAgentFocus(msg.agent, withSummary(fmt.Sprintf("%s on %s asks", label, issueID), parsed.Summary))
	}
	return batchCmds(append(reload, report)...)
}

func withSummary(head, summary string) string {
	if summary = strings.TrimSpace(summary); summary == "" {
		return head
	}
	return head + ": " + summary
}

// offerAgentFocus asks whether to bring the agent to the front, in the action
// modal, only on Detail or the Agents tab with nothing else open: no key
// typed there is text. Anywhere else the question could take a key from work
// on screen — a board, docs or search query takes typed text with no prefix,
// a question, a menu, help, a drag, a surface above the shell — so a warning
// toast points to the agent dialog and the Agents tab instead.
func (m *Model) offerAgentFocus(a agent.Agent, question string) tea.Cmd {
	onAgentSurface := m.active == mode.Detail || m.active == mode.Agents
	if !onAgentSurface || m.overlayOpen() || m.selMenu != nil || m.press != nil {
		return m.showToast(question+" (answer it from the agent dialog or the Agents tab)", toaster.StyleWarn)
	}
	m.agentFocusOffer = &a
	return m.openActionModal(modal.Config{
		Title:       "The agent asks",
		Message:     displaytext.OneLine(question) + " Bring it to the front?",
		ConfirmText: "Focus",
	})
}

// confirmAgentFocus closes the focus offer and brings its agent to the front.
func (m *Model) confirmAgentFocus() tea.Cmd {
	a := *m.agentFocusOffer
	m.showActionModal = false
	m.agentFocusOffer = nil
	return m.focusOneAgent(a)
}

// parseAskReport reads the status line of a reply: the last line that is a
// JSON object with a known status. A fence or backticks around it are
// allowed.
func parseAskReport(reply string) (askReport, bool) {
	lines := strings.Split(reply, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.Trim(strings.TrimSpace(lines[i]), "`")
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var report askReport
		if json.Unmarshal([]byte(line), &report) != nil {
			continue
		}
		switch report.Status {
		case askDone, askFailed, askQuestion:
			return report, true
		}
	}
	return askReport{}, false
}
