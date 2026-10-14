package app

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/displaytext"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/mode/detail"
	issueeditmode "github.com/hk9890/task-manager-ui/internal/mode/issueedit"
	"github.com/hk9890/task-manager-ui/internal/repository"
	uidetail "github.com/hk9890/task-manager-ui/internal/ui/detail"
	"github.com/hk9890/task-manager-ui/internal/ui/modal"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

// The shell half of the issue editor: it opens the screen, reads what the
// screen starts with, builds the write from what the screen holds, and closes
// it. The screen itself writes nothing (internal/mode/issueedit).

// newIssuePriority is the priority a new issue starts on.
const newIssuePriority = 2

// editorTarget is what the editor on screen writes to.
type editorTarget struct {
	kind      issueeditmode.Kind
	issueID   string
	commentID string
}

// editorLoadedMsg carries what an issue editor opens with. seq is the request
// it answers: a key, a click or a wheel notch in between drops it.
type editorLoadedMsg struct {
	seq      int
	kind     issueeditmode.Kind
	closing  bool
	issue    domain.IssueDetail
	catalogs repository.Catalogs
	err      error
}

// editorSavedMsg carries the result of a save. wrote says the store changed,
// also when err is set: a close writes the other fields first.
type editorSavedMsg struct {
	createdID string
	closed    bool
	noChange  bool
	wrote     bool
	err       error
}

// requestEditor starts a request to open the editor and returns its sequence.
func (m *Model) requestEditor() int {
	m.editorSeq++
	m.pendingEditor = m.editorSeq
	return m.editorSeq
}

// openNewIssue opens the editor empty, on the types the store has.
func (m *Model) openNewIssue() tea.Cmd {
	seq := m.requestEditor()
	ctx, repo := m.ctx, m.services.Repo
	return m.scoped(func() tea.Msg {
		catalogs, err := repo.Catalogs(ctx)
		return editorLoadedMsg{seq: seq, kind: issueeditmode.KindCreate, catalogs: catalogs, err: err}
	})
}

// openIssueEditor opens the editor on the selected issue.
func (m *Model) openIssueEditor() tea.Cmd {
	issueID, ok := m.selectedIssueID()
	if !ok {
		return m.showToast("No selected issue to edit", toaster.StyleWarn)
	}
	return m.loadIssueEditor(editorLoadedMsg{}, issueID)
}

// openIssueClose opens the editor on the selected issue with the status on
// closed and the focus on the reason. It writes nothing by itself.
func (m *Model) openIssueClose() tea.Cmd {
	issueID, ok := m.selectedIssueID()
	if !ok {
		return m.showToast("No selected issue to close", toaster.StyleWarn)
	}
	return m.loadIssueEditor(editorLoadedMsg{closing: true}, issueID)
}

// loadIssueEditor reads the issue and the catalogs the editor opens with. The
// issue is always read again: a browse selection holds a summary only, and an
// editor opened on it would write an empty description.
func (m *Model) loadIssueEditor(loaded editorLoadedMsg, issueID string) tea.Cmd {
	loaded.seq = m.requestEditor()
	loaded.kind = issueeditmode.KindEdit
	ctx, repo := m.ctx, m.services.Repo
	return m.scoped(func() tea.Msg {
		loaded.issue, loaded.err = repo.Issue(ctx, issueID)
		if loaded.err == nil {
			loaded.catalogs, loaded.err = repo.Catalogs(ctx)
		}
		return loaded
	})
}

// openCommentAdd opens the editor with the text field alone, for a new
// comment on the selected issue.
func (m *Model) openCommentAdd() tea.Cmd {
	selection := m.currentSelection()
	if selection == nil || selection.Issue.ID == "" {
		return m.showToast("No selected issue to comment on", toaster.StyleWarn)
	}
	m.showEditor(editorTarget{kind: issueeditmode.KindCommentAdd, issueID: selection.Issue.ID}, issueeditmode.Options{
		Subtitle: selection.Issue.Title,
	})
	return nil
}

// openCommentEdit opens the editor with the text field alone, on the comment
// Detail has selected. The issue is the one Detail shows, which a browse
// selection that moved under it no longer names.
func (m *Model) openCommentEdit() tea.Cmd {
	comment, ok := m.detail.SelectedComment()
	if !ok {
		return nil
	}
	issue := m.detail.Detail.Summary
	m.showEditor(editorTarget{kind: issueeditmode.KindCommentEdit, issueID: issue.ID, commentID: comment.ID}, issueeditmode.Options{
		Subtitle: issue.Title,
		Values:   issueeditmode.Values{Text: comment.Body},
	})
	return nil
}

// commentDelete is the comment the delete question on screen is about.
type commentDelete struct {
	issueID   string
	commentID string
}

// commentDeletedMsg carries the result of a comment delete.
type commentDeletedMsg struct {
	issueID string
	err     error
}

// confirmCommentDelete asks, in the action modal, whether to delete the
// comment Detail has selected. It writes nothing by itself.
func (m *Model) confirmCommentDelete() tea.Cmd {
	comment, ok := m.detail.SelectedComment()
	if !ok {
		return nil
	}
	m.commentDelete = commentDelete{issueID: m.detail.Detail.Summary.ID, commentID: comment.ID}
	m.actionModal = modal.NewWithKeys(modal.Config{
		Title:          "Delete comment",
		Message:        displaytext.OneLine(fmt.Sprintf("%s · %s", comment.Author, uidetail.FormatTime(comment.CreatedAt))),
		ConfirmText:    "Delete",
		ConfirmVariant: modal.ButtonDanger,
	}, modal.BindingsFromConfig(m.keys))
	m.actionModal.SetSize(m.width, m.height)
	m.showActionModal = true
	return m.actionModal.Init()
}

// deleteComment closes the delete question and deletes the comment it was
// about.
//
// Uncancellable on purpose, as the save of the editor is.
func (m *Model) deleteComment() tea.Cmd {
	target, repo := m.commentDelete, m.services.Repo
	m.showActionModal = false
	m.commentDelete = commentDelete{}
	return m.scoped(func() tea.Msg {
		err := repo.DeleteComment(context.Background(), target.issueID, target.commentID)
		return commentDeletedMsg{issueID: target.issueID, err: err}
	})
}

func (m *Model) handleCommentDeleted(msg commentDeletedMsg) tea.Cmd {
	if msg.err != nil {
		return m.showToast(fmt.Sprintf("Failed to delete the comment: %v", msg.err), toaster.StyleError)
	}
	done := m.showToast(fmt.Sprintf("Deleted comment on %s", msg.issueID), toaster.StyleSuccess)
	// On a watched store the watch reloads after a write.
	if m.storeWatched() {
		return done
	}
	m.markBrowseSurfacesDirty()
	return batchCmds(m.reloadDetailAfterMutationCmd(msg.issueID), m.maybeAutoRefreshActiveSurfaceCmd(), done)
}

// showEditor puts the editor on screen in the place of the surface it was
// opened from.
func (m *Model) showEditor(target editorTarget, opts issueeditmode.Options) {
	opts.Kind = target.kind
	opts.IssueID = target.issueID
	m.editorReturn = m.active
	m.editorTarget = target
	m.active = mode.IssueEdit
	m.editor.SetSize(m.width, m.height)
	m.editor.Open(opts)
}

// closeEditor returns to the surface the editor was opened from. A selection
// that landed under the editor started no detail load, so the surface below
// catches up here.
func (m *Model) closeEditor() tea.Cmd {
	m.active = m.editorReturn
	return m.ensureDetailForCurrentSelectionCmd()
}

func (m *Model) handleEditorLoaded(msg editorLoadedMsg) tea.Cmd {
	if msg.seq != m.pendingEditor {
		return nil
	}
	m.pendingEditor = 0
	if msg.err != nil {
		return m.showToast(fmt.Sprintf("Failed to open the editor: %v", msg.err), toaster.StyleError)
	}
	// The delete question of a comment is asked with a Cmd, so it can open
	// after the key that asked for the editor. The editor does not open under
	// an overlay.
	if m.overlayOpen() {
		return nil
	}

	issue := msg.issue.Summary
	if msg.closing && issue.Status == domain.StatusClosed {
		return m.showToast(fmt.Sprintf("Issue %s is closed", issue.ID), toaster.StyleWarn)
	}

	opts := issueeditmode.Options{
		Subtitle: "a new issue",
		Values:   issueeditmode.Values{Priority: newIssuePriority},
		Close:    msg.closing,
	}
	for _, status := range msg.catalogs.Statuses {
		opts.Statuses = append(opts.Statuses, status.Name)
	}
	for _, issueType := range msg.catalogs.Types {
		opts.Types = append(opts.Types, issueType.Name)
	}
	if msg.kind == issueeditmode.KindEdit {
		opts.Subtitle = issue.Title
		opts.Values = issueeditmode.Values{
			Title:    issue.Title,
			Status:   issue.Status,
			Type:     issue.Type,
			Priority: issue.Priority,
			Assignee: issue.Assignee,
			Labels:   strings.Join(issue.Labels, ", "),
			Reason:   msg.issue.CloseReason,
			Text:     msg.issue.Description,
		}
	}
	m.showEditor(editorTarget{kind: msg.kind, issueID: issue.ID}, opts)
	return nil
}

// submitEditor starts the write of what the editor holds. The editor has
// taken no key since it asked and takes none until the result is back, so the
// result finds the typed values on screen.
func (m *Model) submitEditor(msg issueeditmode.SubmitMsg) tea.Cmd {
	m.editor.SetSaving(true)
	m.editor.SetError("")
	repo, target := m.services.Repo, m.editorTarget
	return m.scoped(func() tea.Msg { return saveEditor(repo, target, msg.Before, msg.After) })
}

// handleEditorSaved ends a save. A refused write leaves the editor open with
// the typed values and shows the refusal on it.
func (m *Model) handleEditorSaved(msg editorSavedMsg) tea.Cmd {
	issueID := m.editorTarget.issueID
	// On a watched store the watch reloads after a write.
	reload := msg.wrote && !m.storeWatched()

	var cmds []tea.Cmd
	if reload {
		m.markBrowseSurfacesDirty()
		cmds = append(cmds, m.reloadDetailAfterMutationCmd(issueID))
	}
	if msg.err != nil {
		refusal := msg.err.Error()
		// Only a close writes before the write that was refused.
		if msg.wrote {
			refusal += "\nThe other changes are saved."
		}
		m.editor.SetSaving(false)
		m.editor.SetError(refusal)
		return batchCmds(cmds...)
	}

	// A comment written now is the newest, and Detail draws the newest first.
	kind := m.editorTarget.kind
	if msg.wrote && (kind == issueeditmode.KindCommentAdd || kind == issueeditmode.KindCommentEdit) {
		m.detail.SelectComment(0)
	}

	cmds = append(cmds, m.closeEditor())
	if reload {
		cmds = append(cmds, m.maybeAutoRefreshActiveSurfaceCmd())
	}

	done, style := "", toaster.StyleSuccess
	switch {
	case msg.noChange:
		done, style = fmt.Sprintf("No changes saved for issue %s", issueID), toaster.StyleInfo
	case kind == issueeditmode.KindCreate:
		done = fmt.Sprintf("Created issue %s", msg.createdID)
	case kind == issueeditmode.KindCommentAdd:
		done = fmt.Sprintf("Added comment to %s", issueID)
	case kind == issueeditmode.KindCommentEdit:
		done = fmt.Sprintf("Edited comment on %s", issueID)
	case msg.closed:
		done = fmt.Sprintf("Closed issue %s", issueID)
	default:
		done = fmt.Sprintf("Updated issue %s", issueID)
	}
	return batchCmds(append(cmds, m.showToast(done, style))...)
}

// saveEditor writes what the editor holds. It checks nothing itself: an empty
// title, an empty comment, a bad label and a hook refusal come back from the
// store and are shown on the screen.
//
// Uncancellable on purpose: this is the write of what the operator typed.
func saveEditor(repo repository.Repository, target editorTarget, before, after issueeditmode.Values) editorSavedMsg {
	ctx := context.Background()
	switch target.kind {
	case issueeditmode.KindCreate:
		priority := after.Priority
		result, err := repo.CreateIssue(ctx, domain.CreateIssueInput{
			Title:       strings.TrimSpace(after.Title),
			Description: after.Text,
			Type:        after.Type,
			Priority:    &priority,
			Assignee:    strings.TrimSpace(after.Assignee),
			Labels:      parseCommaList(after.Labels),
		})
		return editorSavedMsg{createdID: result.IssueID, wrote: err == nil, err: err}
	case issueeditmode.KindCommentAdd:
		err := repo.AddComment(ctx, target.issueID, domain.AddCommentInput{Body: after.Text})
		return editorSavedMsg{wrote: err == nil, err: err}
	case issueeditmode.KindCommentEdit:
		if after.Text == before.Text {
			return editorSavedMsg{noChange: true}
		}
		err := repo.EditComment(ctx, target.issueID, target.commentID, domain.EditCommentInput{Body: after.Text})
		return editorSavedMsg{wrote: err == nil, err: err}
	}

	input, changed, closing := issueUpdate(before, after)
	if !changed && !closing {
		return editorSavedMsg{noChange: true}
	}
	if changed {
		if err := repo.UpdateIssue(ctx, target.issueID, input); err != nil {
			return editorSavedMsg{err: err}
		}
	}
	if closing {
		// The store takes a reason on a close only, and refuses every other
		// change to a closed issue: the other fields are written first.
		if err := repo.CloseIssue(ctx, target.issueID, domain.CloseIssueInput{Reason: strings.TrimSpace(after.Reason)}); err != nil {
			return editorSavedMsg{wrote: changed, err: err}
		}
	}
	return editorSavedMsg{closed: closing, wrote: true}
}

// issueUpdate is the difference between what the editor opened with and what
// it holds, as one update. closing says the status was stepped to closed: the
// status is then left out, because the close is a write of its own.
func issueUpdate(before, after issueeditmode.Values) (input domain.UpdateIssueInput, changed, closing bool) {
	closing = after.Status == domain.StatusClosed && before.Status != domain.StatusClosed

	text := func(field **string, was, is string) {
		if is != was {
			*field = &is
			changed = true
		}
	}
	text(&input.Title, strings.TrimSpace(before.Title), strings.TrimSpace(after.Title))
	if !closing {
		text(&input.Status, before.Status, after.Status)
	}
	text(&input.Type, before.Type, after.Type)
	text(&input.Assignee, strings.TrimSpace(before.Assignee), strings.TrimSpace(after.Assignee))
	text(&input.Description, before.Text, after.Text)

	if after.Priority != before.Priority {
		input.Priority = &after.Priority
		changed = true
	}

	labels := parseCommaList(after.Labels)
	if !slices.Equal(labelSet(labels), labelSet(parseCommaList(before.Labels))) {
		if len(labels) == 0 {
			input.ClearLabels = true
		} else {
			input.Labels = labels
		}
		changed = true
	}
	return input, changed, closing
}

// labelSet is labels in one order with each label once.
func labelSet(labels []string) []string {
	return slices.Compact(slices.Sorted(slices.Values(labels)))
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

// reloadDetailAfterMutationCmd starts the post-mutation detail reload, pairing
// BeginLoad with loadDetailCmd as every other load site does. Without the
// pairing the header shows no spinner while the read runs, and a browse
// selection that moves in that window makes the response fail the target-id
// guard in update() and be discarded.
func (m *Model) reloadDetailAfterMutationCmd(issueID string) tea.Cmd {
	if strings.TrimSpace(issueID) == "" {
		return nil
	}
	m.detail.BeginLoad(issueID, detail.BeginLoadOptions{})
	return m.loadDetail(issueID)
}

// editorHints is the issue editor's own legend: the shell footer is not drawn
// while it is up. The keys that move between the fields are built in, so the
// legend names the keys themselves.
func editorHints(keys config.ResolvedKeyBindings, kind issueeditmode.Kind) []styles.KeyHint {
	primary := keys.DisplayPrimary
	hints := []styles.KeyHint{
		{Key: "tab/shift+tab", Desc: "field"},
		{Key: primary(config.ShellContext, config.ShellActionSaveEdit), Desc: "save"},
		{Key: primary(config.ShellContext, config.ShellActionEscape), Desc: "cancel"},
	}
	if kind == issueeditmode.KindCreate || kind == issueeditmode.KindEdit {
		hints = append(hints, styles.KeyHint{Key: "left/right", Desc: "change"})
	}
	return append(hints, styles.KeyHint{Key: primary(config.ShellContext, config.ShellActionQuit), Desc: "quit"})
}
