package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/ui/modal"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

type mutationKind string

const (
	mutationCreate   mutationKind = "create"
	mutationUpdate   mutationKind = "update"
	mutationClose    mutationKind = "close"
	mutationComment  mutationKind = "comment"
	mutationStatus   mutationKind = "status"
	mutationPriority mutationKind = "priority"
)

type mutationCatalogsLoadedMsg struct {
	kind     mutationKind
	issue    domain.IssueSummary
	statuses []domain.StatusOption
	types    []domain.TypeOption
	labels   []domain.LabelOption
	err      error
}

type mutationResultMsg struct {
	kind      mutationKind
	issueID   string
	createdID string
	noChange  bool
	err       error
}

type mutationDialogState struct {
	kind        mutationKind
	issue       domain.IssueSummary
	statusNames map[string]struct{}
	typeNames   map[string]struct{}
	labelNames  map[string]struct{}
	statusList  string
	typeList    string
	labelList   string
}

// pendingDialogGuard tracks an in-flight async dialog-open so that a key
// press (ESC or otherwise) arriving before the catalog response is delivered
// can cancel the pending open instead of causing the dialog to appear over the
// wrong mode. It is keyed on kind (not issue ID) so that the create path,
// which uses an empty IssueSummary, is handled correctly.
type pendingDialogGuard struct {
	active bool
	kind   mutationKind
}

func loadMutationCatalogsCmd(services Services, kind mutationKind, issue domain.IssueSummary) tea.Cmd {
	return func() tea.Msg {
		catalogs, err := services.Repo.Catalogs(context.Background())
		if err != nil {
			return mutationCatalogsLoadedMsg{kind: kind, issue: issue, err: fmt.Errorf("catalogs: %w", err)}
		}

		return mutationCatalogsLoadedMsg{kind: kind, issue: issue, statuses: catalogs.Statuses, types: catalogs.Types, labels: catalogs.Labels}
	}
}

func buildMutationDialog(kind mutationKind, issue domain.IssueSummary, statuses []domain.StatusOption, types []domain.TypeOption, labels []domain.LabelOption) mutationDialogState {
	statusNames := make(map[string]struct{}, len(statuses))
	typeNames := make(map[string]struct{}, len(types))
	labelNames := make(map[string]struct{}, len(labels))

	statusList := make([]string, 0, len(statuses))
	typeList := make([]string, 0, len(types))
	labelList := make([]string, 0, len(labels))

	for _, option := range statuses {
		name := strings.TrimSpace(option.Name)
		if name == "" {
			continue
		}
		statusNames[name] = struct{}{}
		statusList = append(statusList, name)
	}

	for _, option := range types {
		name := strings.TrimSpace(option.Name)
		if name == "" {
			continue
		}
		typeNames[name] = struct{}{}
		typeList = append(typeList, name)
	}

	for _, option := range labels {
		name := strings.TrimSpace(option.Name)
		if name == "" {
			continue
		}
		labelNames[name] = struct{}{}
		labelList = append(labelList, name)
	}

	return mutationDialogState{
		kind:        kind,
		issue:       issue,
		statusNames: statusNames,
		typeNames:   typeNames,
		labelNames:  labelNames,
		statusList:  strings.Join(statusList, ", "),
		typeList:    strings.Join(typeList, ", "),
		labelList:   strings.Join(labelList, ", "),
	}
}

func mutationModal(state mutationDialogState, keys config.ResolvedKeyBindings) modal.Model {
	switch state.kind {
	case mutationCreate:
		return modal.NewWithKeys(modal.Config{
			Title:       "Create Issue",
			Message:     fmt.Sprintf("Inline quick-create flow (rich editing stays in external editor).\nTypes: %s\nLabels: %s", emptyFallback(state.typeList, "(none)"), emptyFallback(state.labelList, "(none)")),
			ConfirmText: "Create",
			MinWidth:    92,
			Required:    false,
			Inputs: []modal.InputConfig{
				{Key: "title", Label: "Title", Placeholder: "Issue title"},
				{Key: "type", Label: "Type", Placeholder: emptyFallback(state.typeList, "task")},
				{Key: "priority", Label: "Priority", Placeholder: "0-4"},
				{Key: "assignee", Label: "Assignee", Placeholder: "username"},
				{Key: "labels", Label: "Labels", Placeholder: "comma,separated"},
				{Key: "description", Label: "Description", Placeholder: "Short description"},
			},
		}, modal.BindingsFromConfig(keys))
	case mutationUpdate:
		return modal.NewWithKeys(modal.Config{
			Title:       fmt.Sprintf("Update Issue %s", state.issue.ID),
			Message:     fmt.Sprintf("Quick metadata update.\nStatuses: %s\nTypes: %s\nLabels: %s", emptyFallback(state.statusList, "(none)"), emptyFallback(state.typeList, "(none)"), emptyFallback(state.labelList, "(none)")),
			ConfirmText: "Update",
			MinWidth:    92,
			Required:    false,
			Inputs: []modal.InputConfig{
				{Key: "title", Label: "Title", Value: state.issue.Title, Placeholder: "Leave unchanged"},
				{Key: "status", Label: "Status", Value: state.issue.Status, Placeholder: emptyFallback(state.statusList, state.issue.Status)},
				{Key: "type", Label: "Type", Value: state.issue.Type, Placeholder: emptyFallback(state.typeList, state.issue.Type)},
				{Key: "priority", Label: "Priority", Value: strconv.Itoa(state.issue.Priority), Placeholder: "0-4"},
				{Key: "assignee", Label: "Assignee", Value: state.issue.Assignee, Placeholder: "username"},
				{Key: "labels", Label: "Labels", Value: strings.Join(state.issue.Labels, ","), Placeholder: "comma,separated"},
			},
		}, modal.BindingsFromConfig(keys))
	case mutationClose:
		return modal.NewWithKeys(modal.Config{
			Title:          fmt.Sprintf("Close Issue %s", state.issue.ID),
			Message:        "Provide an optional close reason.",
			ConfirmText:    "Close",
			ConfirmVariant: modal.ButtonDanger,
			Required:       false,
			MinWidth:       72,
			Inputs: []modal.InputConfig{
				{Key: "reason", Label: "Reason", Placeholder: "completed"},
			},
		}, modal.BindingsFromConfig(keys))
	case mutationComment:
		return modal.NewWithKeys(modal.Config{
			Title:       fmt.Sprintf("Comment on %s", state.issue.ID),
			Message:     "Add a comment for the selected issue.",
			ConfirmText: "Add comment",
			Required:    false,
			MinWidth:    72,
			Inputs: []modal.InputConfig{
				{Key: "body", Label: "Comment", Placeholder: "Comment text"},
			},
		}, modal.BindingsFromConfig(keys))
	case mutationStatus:
		return modal.NewWithKeys(modal.Config{
			Title:         fmt.Sprintf("Update Status %s", state.issue.ID),
			Message:       fmt.Sprintf("Set the issue status. Available: %s", emptyFallback(state.statusList, "(none)")),
			ConfirmText:   "Update",
			MinWidth:      72,
			Required:      true,
			SubmitOnEnter: true,
			Inputs: []modal.InputConfig{
				{Key: "status", Label: "Status", Value: state.issue.Status, Placeholder: emptyFallback(state.statusList, state.issue.Status)},
			},
		}, modal.BindingsFromConfig(keys))
	case mutationPriority:
		return modal.NewWithKeys(modal.Config{
			Title:         fmt.Sprintf("Update Priority %s", state.issue.ID),
			Message:       "Set the issue priority (0-4).",
			ConfirmText:   "Update",
			MinWidth:      72,
			Required:      true,
			SubmitOnEnter: true,
			Inputs: []modal.InputConfig{
				{Key: "priority", Label: "Priority", Value: strconv.Itoa(state.issue.Priority), Placeholder: "0-4"},
			},
		}, modal.BindingsFromConfig(keys))
	default:
		return modal.NewWithKeys(modal.Config{Title: "Action", Message: "Unsupported action", Required: false}, modal.BindingsFromConfig(keys))
	}
}

// handleMutationResult processes mutationResultMsg in Update.
func (m Model) handleMutationResult(modeCmd tea.Cmd, msg mutationResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, batchCmds(modeCmd, m.showToast(msg.err.Error(), toaster.StyleError))
	}

	if msg.noChange {
		return m, batchCmds(modeCmd, m.showToast(fmt.Sprintf("No changes saved for issue %s", msg.issueID), toaster.StyleInfo))
	}

	m.markBrowseSurfacesDirty()

	switch msg.kind {
	case mutationCreate:
		return m, batchCmds(modeCmd,
			m.showToast(fmt.Sprintf("Created issue %s", emptyFallback(msg.createdID, "(unknown)")), toaster.StyleSuccess),
			m.maybeAutoRefreshActiveSurfaceCmd(),
		)
	case mutationUpdate:
		return m, batchCmds(modeCmd,
			m.showToast(fmt.Sprintf("Updated issue %s", msg.issueID), toaster.StyleSuccess),
			loadDetailCmd(m.services, msg.issueID),
			m.maybeAutoRefreshActiveSurfaceCmd(),
		)
	case mutationClose:
		return m, batchCmds(modeCmd,
			m.showToast(fmt.Sprintf("Closed issue %s", msg.issueID), toaster.StyleSuccess),
			loadDetailCmd(m.services, msg.issueID),
			m.maybeAutoRefreshActiveSurfaceCmd(),
		)
	case mutationComment:
		return m, batchCmds(modeCmd,
			m.showToast(fmt.Sprintf("Added comment to %s", msg.issueID), toaster.StyleSuccess),
			loadDetailCmd(m.services, msg.issueID),
			m.maybeAutoRefreshActiveSurfaceCmd(),
		)
	case mutationStatus:
		return m, batchCmds(modeCmd,
			m.showToast(fmt.Sprintf("Updated issue status for %s", msg.issueID), toaster.StyleSuccess),
			loadDetailCmd(m.services, msg.issueID),
			m.maybeAutoRefreshActiveSurfaceCmd(),
		)
	case mutationPriority:
		return m, batchCmds(modeCmd,
			m.showToast(fmt.Sprintf("Updated issue priority for %s", msg.issueID), toaster.StyleSuccess),
			loadDetailCmd(m.services, msg.issueID),
			m.maybeAutoRefreshActiveSurfaceCmd(),
		)
	default:
		return m, modeCmd
	}
}

func submitMutationCmd(services Services, state mutationDialogState, values map[string]string) tea.Cmd {
	return func() tea.Msg {
		switch state.kind {
		case mutationCreate:
			title := strings.TrimSpace(values["title"])
			if title == "" {
				return mutationResultMsg{kind: mutationCreate, err: fmt.Errorf("create issue failed: title is required")}
			}

			priority, err := parsePriority(values["priority"])
			if err != nil {
				return mutationResultMsg{kind: mutationCreate, err: fmt.Errorf("create issue failed: %w", err)}
			}

			labels := parseCommaList(values["labels"])
			result, err := services.Repo.CreateIssue(context.Background(), domain.CreateIssueInput{
				Title:       title,
				Description: strings.TrimSpace(values["description"]),
				Type:        strings.TrimSpace(values["type"]),
				Priority:    priority,
				Assignee:    strings.TrimSpace(values["assignee"]),
				Labels:      labels,
			})
			if err != nil {
				return mutationResultMsg{kind: mutationCreate, err: fmt.Errorf("create issue failed: %w", err)}
			}

			return mutationResultMsg{kind: mutationCreate, createdID: result.IssueID}
		case mutationUpdate:
			status := strings.TrimSpace(values["status"])
			if status != "" {
				if _, ok := state.statusNames[status]; len(state.statusNames) > 0 && !ok {
					return mutationResultMsg{kind: mutationUpdate, issueID: state.issue.ID, err: fmt.Errorf("update issue failed: unknown status %q", status)}
				}
			}

			issueType := strings.TrimSpace(values["type"])
			if issueType != "" {
				if _, ok := state.typeNames[issueType]; len(state.typeNames) > 0 && !ok {
					return mutationResultMsg{kind: mutationUpdate, issueID: state.issue.ID, err: fmt.Errorf("update issue failed: unknown type %q", issueType)}
				}
			}

			priority, err := parseRequiredPriority(values["priority"])
			if err != nil {
				return mutationResultMsg{kind: mutationUpdate, issueID: state.issue.ID, err: fmt.Errorf("update issue failed: %w", err)}
			}

			labels := parseCommaList(values["labels"])
			for _, label := range labels {
				if _, ok := state.labelNames[label]; len(state.labelNames) > 0 && !ok {
					return mutationResultMsg{kind: mutationUpdate, issueID: state.issue.ID, err: fmt.Errorf("update issue failed: unknown label %q", label)}
				}
			}

			title := strings.TrimSpace(values["title"])
			assignee := strings.TrimSpace(values["assignee"])
			input := domain.UpdateIssueInput{}
			if title != "" {
				input.Title = &title
			}
			if status != "" {
				input.Status = &status
			}
			if issueType != "" {
				input.Type = &issueType
			}
			if priority != nil {
				input.Priority = priority
			}
			// Diff against the original assignee rather than gating on non-empty, so
			// clearing the pre-filled field unassigns the issue (sends Assignee=""),
			// instead of being silently treated as "no change".
			if assignee != strings.TrimSpace(state.issue.Assignee) {
				input.Assignee = &assignee
			}
			if len(labels) > 0 {
				input.Labels = labels
			} else {
				input.ClearLabels = true
			}

			if err := services.Repo.UpdateIssue(context.Background(), state.issue.ID, input); err != nil {
				return mutationResultMsg{kind: mutationUpdate, issueID: state.issue.ID, err: fmt.Errorf("update issue failed: %w", err)}
			}

			return mutationResultMsg{kind: mutationUpdate, issueID: state.issue.ID}
		case mutationClose:
			reason := strings.TrimSpace(values["reason"])
			if err := services.Repo.CloseIssue(context.Background(), state.issue.ID, domain.CloseIssueInput{Reason: reason}); err != nil {
				return mutationResultMsg{kind: mutationClose, issueID: state.issue.ID, err: fmt.Errorf("close issue failed: %w", err)}
			}
			return mutationResultMsg{kind: mutationClose, issueID: state.issue.ID}
		case mutationComment:
			body := strings.TrimSpace(values["body"])
			if body == "" {
				return mutationResultMsg{kind: mutationComment, issueID: state.issue.ID, err: fmt.Errorf("add comment failed: body is required")}
			}
			if err := services.Repo.AddComment(context.Background(), state.issue.ID, domain.AddCommentInput{Body: body}); err != nil {
				return mutationResultMsg{kind: mutationComment, issueID: state.issue.ID, err: fmt.Errorf("add comment failed: %w", err)}
			}
			return mutationResultMsg{kind: mutationComment, issueID: state.issue.ID}
		case mutationStatus:
			status := strings.TrimSpace(values["status"])
			if status == "" {
				return mutationResultMsg{kind: mutationStatus, issueID: state.issue.ID, err: fmt.Errorf("update status failed: status is required")}
			}
			if _, ok := state.statusNames[status]; len(state.statusNames) > 0 && !ok {
				return mutationResultMsg{kind: mutationStatus, issueID: state.issue.ID, err: fmt.Errorf("update status failed: unknown status %q", status)}
			}
			if status == strings.TrimSpace(state.issue.Status) {
				return mutationResultMsg{kind: mutationStatus, issueID: state.issue.ID, noChange: true}
			}

			if err := services.Repo.UpdateIssue(context.Background(), state.issue.ID, domain.UpdateIssueInput{Status: &status}); err != nil {
				return mutationResultMsg{kind: mutationStatus, issueID: state.issue.ID, err: fmt.Errorf("update status failed: %w", err)}
			}
			return mutationResultMsg{kind: mutationStatus, issueID: state.issue.ID}
		case mutationPriority:
			priority, err := parseRequiredPriority(values["priority"])
			if err != nil {
				return mutationResultMsg{kind: mutationPriority, issueID: state.issue.ID, err: fmt.Errorf("update priority failed: %w", err)}
			}
			if priority == nil {
				return mutationResultMsg{kind: mutationPriority, issueID: state.issue.ID, err: fmt.Errorf("update priority failed: priority is required")}
			}
			// Range (0..4) is enforced by parseRequiredPriority above.
			if *priority == state.issue.Priority {
				return mutationResultMsg{kind: mutationPriority, issueID: state.issue.ID, noChange: true}
			}

			if err := services.Repo.UpdateIssue(context.Background(), state.issue.ID, domain.UpdateIssueInput{Priority: priority}); err != nil {
				return mutationResultMsg{kind: mutationPriority, issueID: state.issue.ID, err: fmt.Errorf("update priority failed: %w", err)}
			}
			return mutationResultMsg{kind: mutationPriority, issueID: state.issue.ID}
		default:
			return mutationResultMsg{kind: state.kind, issueID: state.issue.ID, err: fmt.Errorf("unsupported mutation action")}
		}
	}
}

func parseCommaList(value string) []string {
	parts := strings.Split(value, ",")
	labels := make([]string, 0, len(parts))
	for _, part := range parts {
		label := strings.TrimSpace(part)
		if label == "" {
			continue
		}
		labels = append(labels, label)
	}
	return labels
}

func parsePriority(value string) (*int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}

	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return nil, fmt.Errorf("priority must be an integer")
	}
	if parsed < 0 || parsed > 4 {
		return nil, fmt.Errorf("priority must be between 0 and 4")
	}

	return &parsed, nil
}

func parseRequiredPriority(value string) (*int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, fmt.Errorf("priority is required")
	}

	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return nil, fmt.Errorf("priority must be an integer")
	}
	if parsed < 0 || parsed > 4 {
		return nil, fmt.Errorf("priority must be between 0 and 4")
	}

	return &parsed, nil
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
