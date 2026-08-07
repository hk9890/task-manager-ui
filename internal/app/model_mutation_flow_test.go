package app

// Happy-path mutation flows through the shell dialogs (create, update, close,
// comment) and the guard that cancels a dialog whose catalog load is still in
// flight. Mutation *error* handling lives in mutation_errors_test.go.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/ui/modal"
)

func TestModelCreateIssueFlowUsesRepositoryCatalogsAndCreateIssue(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedCatalogs(
		[]domain.StatusOption{{Name: "open"}, {Name: "in_progress"}},
		[]domain.TypeOption{{Name: "task"}, {Name: "bug"}},
		[]domain.LabelOption{{Name: "ui"}, {Name: "infra"}},
	)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = next.(Model)

	if cmd == nil {
		t.Fatalf("expected command for create flow")
	}

	next, cmd = m.Update(cmd())
	m = next.(Model)

	if !m.showActionModal {
		t.Fatalf("expected action modal to open for create")
	}

	submit := modal.SubmitMsg{Values: map[string]string{
		"title":       "Create from modal",
		"type":        "task",
		"priority":    "2",
		"assignee":    "hans",
		"labels":      "ui,infra",
		"description": "created from test",
	}}
	next, cmd = m.Update(submit)
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected submit mutation command")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)

	if !gw.hasCatalogsCall() {
		t.Fatalf("expected catalogs to be queried, calls=%#v", gw.Calls())
	}

	if !gw.hasCreateIssueCall() {
		t.Fatalf("expected create issue repository call, calls=%#v", gw.Calls())
	}
}

func TestModelUpdateCloseAndCommentFlowsUseRepositoryWrites(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1, func(i *memoryrepo.Issue) { i.Labels = []string{"ui"} })
	gw.seedInProgress("tm-2", "In progress", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1, Labels: []string{"ui"}}})
	gw.seedCatalogs(
		[]domain.StatusOption{{Name: "open"}, {Name: "in_progress"}},
		[]domain.TypeOption{{Name: "task"}, {Name: "bug"}},
		[]domain.LabelOption{{Name: "ui"}, {Name: "infra"}},
	)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = next.(Model)

	if cmd == nil {
		t.Fatalf("expected command for update flow")
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	next, cmd = m.Update(modal.SubmitMsg{Values: map[string]string{
		"title":    "Updated title",
		"status":   "in_progress",
		"type":     "task",
		"priority": "2",
		"assignee": "hans",
		"labels":   "ui,infra",
	}})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected update mutation command")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected modal init command for close flow")
	}
	next, cmd = m.Update(modal.SubmitMsg{Values: map[string]string{"reason": "done"}})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected close mutation command")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected modal init command for comment flow")
	}
	next, cmd = m.Update(modal.SubmitMsg{Values: map[string]string{"body": "looks good"}})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected comment mutation command")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)

	if !gw.hasUpdateIssueCall() {
		t.Fatalf("expected update issue call, calls=%#v", gw.Calls())
	}
	if !gw.hasCloseIssueCall() {
		t.Fatalf("expected close issue call, calls=%#v", gw.Calls())
	}
	if !gw.hasAddCommentCall() {
		t.Fatalf("expected add comment call, calls=%#v", gw.Calls())
	}
}

// TestModelMutationModalsOpenWithoutCatalogDecodeToast verifies that c/u/a keys
// open the respective action modals without triggering a "Failed to load
// mutation catalogs" toast. This replaces
// TestModelEmbeddedFixtureMutationModalsOpenWithoutCatalogDecodeToast
// (which used real taskmgr+fixture).
func TestModelMutationModalsOpenWithoutCatalogDecodeToast(t *testing.T) {
	gw := newTestRepository()
	gw.seedIssueSummary(domain.IssueSummary{ID: "bwf-2", Title: "Blocked bug for fixture", Status: "blocked", Type: "bug", Priority: 0})
	gw.seedIssueDetail(domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "bwf-2", Title: "Blocked bug for fixture", Status: "blocked", Type: "bug", Priority: 0, Assignee: "bob"},
		Description: "Used to validate blocked/ready and dependency reads.",
	})
	gw.seedCatalogs(
		[]domain.StatusOption{{Name: "open"}, {Name: "blocked"}, {Name: "in_progress"}},
		[]domain.TypeOption{{Name: "task"}, {Name: "bug"}, {Name: "chore"}},
		[]domain.LabelOption{{Name: "fixture"}, {Name: "blocking"}},
	)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.width = 120
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	assertModalOpenWithoutCatalogToast := func(model Model, wantTitle string) {
		t.Helper()
		if !model.showActionModal {
			t.Fatalf("expected action modal %q to open", wantTitle)
		}
		if !strings.Contains(model.actionModal.View(), wantTitle) {
			t.Fatalf("expected modal title %q, got:\n%s", wantTitle, model.actionModal.View())
		}
		if model.toast.Visible() {
			t.Fatalf("expected no toast while opening %q modal, got:\n%s", wantTitle, model.View())
		}
		if strings.Contains(model.View(), "Failed to load mutation catalogs") {
			t.Fatalf("expected no mutation catalog decode toast while opening %q modal, got:\n%s", wantTitle, model.View())
		}
	}

	// 'c' opens Create Issue modal.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected create flow command")
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	assertModalOpenWithoutCatalogToast(m, "Create Issue")

	// Escape closes the modal.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(Model)
	}
	if m.showActionModal {
		t.Fatal("expected create modal to close on escape")
	}

	// 'u' opens Update Issue modal (title includes selected issue ID).
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected update flow command")
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	assertModalOpenWithoutCatalogToast(m, "Update Issue bwf-2")

	// Escape closes the modal.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(Model)
	}
	if m.showActionModal {
		t.Fatal("expected update modal to close on escape")
	}

	// Enter dedicated detail mode so the 'a' (add comment) hotkey works.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Detail {
		t.Fatalf("expected detail mode before comment flow, got %s", m.active)
	}

	// 'a' opens Comment modal.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected comment flow command")
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	assertModalOpenWithoutCatalogToast(m, "Comment on bwf-2")
}

// TestPendingDialogGuardStatusRaceEscCancelsOpen reproduces the async ESC race
// for the Update Status dialog:
//   - Enter on the Status metadata row dispatches an async catalog-load Cmd and
//     sets the pending-dialog guard.
//   - ESC arrives before the catalog response → guard is cleared; ESC is
//     consumed as "cancel the pending open" without popping Detail → Board.
//   - The catalog-loaded message arrives → guard is gone → modal is NOT opened.
//
// Expected: m.active == Detail, m.showActionModal == false.
func TestPendingDialogGuardStatusRaceEscCancelsOpen(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Root", "task", 1)
	gw.seedInProgress("tm-2", "Other", "task", 2)
	gw.seedIssueDetail(domain.IssueDetail{Summary: domain.IssueSummary{ID: "tm-1", Title: "Root", Status: "open", Type: "task", Priority: 1}})
	gw.seedCatalogs(
		[]domain.StatusOption{{Name: "open"}, {Name: "in_progress"}, {Name: "blocked"}},
		[]domain.TypeOption{{Name: "task"}},
		[]domain.LabelOption{},
	)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.width = 140
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	// Navigate to Detail mode.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Detail {
		t.Fatalf("expected Detail mode after pressing 3, got %s", m.active)
	}

	// Navigate to the Metadata pane (Right arrow) and focus the Status row.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	// Press Enter on the Status row — this dispatches the async catalog load and
	// sets the pending-dialog guard. Capture the Cmd but do NOT execute it yet.
	next, catalogLoadCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if catalogLoadCmd == nil {
		t.Fatal("expected async catalog-load Cmd after Enter on Status row")
	}
	if !m.pendingDialog.active {
		t.Fatal("expected pending-dialog guard to be active after dispatching catalog load")
	}
	if m.pendingDialog.kind != mutationStatus {
		t.Fatalf("expected pending-dialog kind=mutationStatus, got %q", m.pendingDialog.kind)
	}

	// ESC arrives during the load window. The guard must be cleared and ESC
	// must NOT pop Detail → Board.
	next, escCmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if escCmd != nil {
		// Drain any follow-up commands (e.g. modeCmd from mode sub-models).
		m = applyMessages(t, m, runBatch(escCmd))
	}

	if m.active != mode.Detail {
		t.Fatalf("ESC during pending-dialog load popped mode to %s; want Detail", m.active)
	}
	if m.pendingDialog.active {
		t.Fatal("expected pending-dialog guard to be cleared after ESC")
	}

	// Now deliver the catalog-loaded message. Because the guard is gone the
	// handler must drop the result without opening the modal.
	catalogMsg := catalogLoadCmd()
	next, afterCmd := m.Update(catalogMsg)
	m = next.(Model)
	if afterCmd != nil {
		m = applyMessages(t, m, runBatch(afterCmd))
	}

	if m.showActionModal {
		t.Fatal("expected no action modal after ESC cancelled the pending-dialog open; got orphaned modal")
	}
	if m.active != mode.Detail {
		t.Fatalf("expected mode to remain Detail after catalog arrival with cancelled guard; got %s", m.active)
	}
}

// TestPendingDialogGuardCreateUpdateRaceEscCancelsOpen reproduces the async ESC
// race for the Create and Update mutation dialogs.
// Create dispatches with an empty IssueSummary (no issue ID), so the guard must
// key on kind, not issue ID.
//
//   - Press the Create-issue key → dispatches async catalog load, sets guard.
//   - ESC arrives before catalog response → guard cleared, no mode switch.
//   - mutationCatalogsLoadedMsg with kind=mutationCreate arrives → dropped, no modal.
//
// The Update path is also checked: guard is set with kind=mutationUpdate.
func TestPendingDialogGuardCreateUpdateRaceEscCancelsOpen(t *testing.T) {
	t.Parallel()

	t.Run("create", func(t *testing.T) {
		t.Parallel()

		gw := newTestRepository()
		gw.seedReady("tm-1", "Root", "task", 1)
		gw.seedCatalogs(
			[]domain.StatusOption{{Name: "open"}, {Name: "in_progress"}},
			[]domain.TypeOption{{Name: "task"}, {Name: "bug"}},
			[]domain.LabelOption{},
		)

		services, err := NewServices(gw, config.Default(), t.TempDir())
		if err != nil {
			t.Fatalf("NewServices returned error: %v", err)
		}

		m := mustNewModel(t, services)
		m.width = 140
		m.height = 34
		m = applyMessages(t, m, runBatch(m.Init()))

		// Press "c" (ShellActionCreateIssue) — dispatches the async catalog load.
		next, catalogLoadCmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
		m = next.(Model)
		if catalogLoadCmd == nil {
			t.Fatal("expected async catalog-load Cmd after Create-issue key")
		}
		if !m.pendingDialog.active {
			t.Fatal("expected pending-dialog guard to be active after dispatching create catalog load")
		}
		if m.pendingDialog.kind != mutationCreate {
			t.Fatalf("expected pending-dialog kind=mutationCreate, got %q", m.pendingDialog.kind)
		}

		// ESC during the load window cancels the pending open.
		next, escCmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = next.(Model)
		if escCmd != nil {
			m = applyMessages(t, m, runBatch(escCmd))
		}

		if m.pendingDialog.active {
			t.Fatal("expected pending-dialog guard cleared after ESC")
		}

		// Deliver the catalog-loaded message. Guard is gone → modal must NOT open.
		// Construct the message directly using the empty-issue-ID shape that create uses.
		catalogMsg := mutationCatalogsLoadedMsg{
			kind:     mutationCreate,
			issue:    domain.IssueSummary{}, // empty ID — the create path
			statuses: []domain.StatusOption{{Name: "open"}, {Name: "in_progress"}},
			types:    []domain.TypeOption{{Name: "task"}, {Name: "bug"}},
			labels:   []domain.LabelOption{},
		}
		next, afterCmd := m.Update(catalogMsg)
		m = next.(Model)
		if afterCmd != nil {
			m = applyMessages(t, m, runBatch(afterCmd))
		}

		if m.showActionModal {
			t.Fatal("expected no action modal after ESC cancelled the create pending-dialog open; got orphaned modal")
		}
	})

	t.Run("update", func(t *testing.T) {
		t.Parallel()

		gw := newTestRepository()
		gw.seedReady("tm-1", "Root", "task", 1)
		gw.seedCatalogs(
			[]domain.StatusOption{{Name: "open"}, {Name: "in_progress"}},
			[]domain.TypeOption{{Name: "task"}, {Name: "bug"}},
			[]domain.LabelOption{},
		)

		services, err := NewServices(gw, config.Default(), t.TempDir())
		if err != nil {
			t.Fatalf("NewServices returned error: %v", err)
		}

		m := mustNewModel(t, services)
		m.width = 140
		m.height = 34
		m = applyMessages(t, m, runBatch(m.Init()))

		// Press "u" (ShellActionUpdateIssue) — dispatches the async catalog load.
		next, catalogLoadCmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
		m = next.(Model)
		if catalogLoadCmd == nil {
			t.Fatal("expected async catalog-load Cmd after Update-issue key")
		}
		if !m.pendingDialog.active {
			t.Fatal("expected pending-dialog guard to be active after dispatching update catalog load")
		}
		if m.pendingDialog.kind != mutationUpdate {
			t.Fatalf("expected pending-dialog kind=mutationUpdate, got %q", m.pendingDialog.kind)
		}

		// ESC during the load window cancels the pending open.
		next, escCmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = next.(Model)
		if escCmd != nil {
			m = applyMessages(t, m, runBatch(escCmd))
		}

		if m.pendingDialog.active {
			t.Fatal("expected pending-dialog guard cleared after ESC")
		}

		// Deliver the catalog-loaded message. Guard is gone → modal must NOT open.
		catalogMsg := mutationCatalogsLoadedMsg{
			kind:     mutationUpdate,
			issue:    domain.IssueSummary{ID: "tm-1"},
			statuses: []domain.StatusOption{{Name: "open"}, {Name: "in_progress"}},
			types:    []domain.TypeOption{{Name: "task"}, {Name: "bug"}},
			labels:   []domain.LabelOption{},
		}
		next, afterCmd := m.Update(catalogMsg)
		m = next.(Model)
		if afterCmd != nil {
			m = applyMessages(t, m, runBatch(afterCmd))
		}

		if m.showActionModal {
			t.Fatal("expected no action modal after ESC cancelled the update pending-dialog open; got orphaned modal")
		}
	})
}

// --- Keyboard-driven dialog path -------------------------------------------
//
// The tests above hand the shell a ready-made modal.SubmitMsg. That proves the
// shell reacts correctly to a submit, but it skips everything a user actually
// does: typing into the input, moving focus, and pressing the key that submits.
// A regression anywhere in that stretch — modal key handling, the shell's
// forwarding of keys while a dialog is open, or the mapping of typed values
// onto the write — would leave every SubmitMsg test green.
//
// These tests drive real tea.KeyMsg values end to end and assert the value that
// reached the store, not merely that a call happened.

// typeIntoDialog sends each rune to the shell.
//
// The returned Cmd is deliberately discarded: the focused textinput answers a
// rune with a cursor-blink tick, and runBatch would invoke it and block the
// test on a timer.
func typeIntoDialog(m Model, text string) Model {
	for _, r := range text {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	return m
}

// pressAndSettle sends one key and drains everything it sets off.
func pressAndSettle(t *testing.T, m Model, msg tea.KeyMsg) Model {
	t.Helper()
	next, cmd := m.Update(msg)
	return applyMessages(t, next.(Model), runBatch(cmd))
}

// openDialog presses a mutation hotkey and feeds in the catalog load that backs
// the dialog, returning a model with the modal open.
//
// The catalog results are applied one at a time with the follow-up Cmds
// discarded, rather than through applyMessages: opening the modal returns
// modal.Init, which is textinput.Blink. Draining that invokes a tick, and each
// blink schedules the next, so a full drain never terminates.
func openDialog(t *testing.T, m Model, hotkey rune) Model {
	t.Helper()

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{hotkey}})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected a catalog-load command from %q", string(hotkey))
	}

	for _, msg := range runBatch(cmd) {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	if !m.showActionModal {
		t.Fatalf("expected the %q dialog to be open", string(hotkey))
	}
	return m
}

func TestCommentDialogKeyboardPathWritesTypedBody(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	m = openDialog(t, m, 'a')
	m = typeIntoDialog(m, "looks good")

	// First Enter advances off the input. Nothing may be written yet — this is
	// the contract that cost a manual verification run to discover.
	m = pressAndSettle(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if gw.hasAddCommentCall() {
		t.Fatal("Enter on the focused input must not submit the comment dialog")
	}
	if !m.showActionModal {
		t.Fatal("expected the dialog to stay open after the field-advance Enter")
	}

	// Second Enter, with the buttons focused, submits.
	m = pressAndSettle(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.showActionModal {
		t.Fatal("expected the dialog to close after submitting")
	}
	if !gw.hasAddCommentCall() {
		t.Fatalf("expected AddComment after the submitting Enter, calls=%#v", gw.Calls())
	}

	detail, err := gw.repo.Issue(t.Context(), "tm-1")
	if err != nil {
		t.Fatalf("reading tm-1 back: %v", err)
	}
	if len(detail.Comments) != 1 {
		t.Fatalf("expected exactly one comment on tm-1, got %d", len(detail.Comments))
	}
	if got := detail.Comments[0].Body; got != "looks good" {
		t.Errorf("typed comment body did not reach the store: got %q, want %q", got, "looks good")
	}
}

func TestCreateDialogKeyboardPathWritesTypedTitle(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	m = openDialog(t, m, 'c')
	m = typeIntoDialog(m, "Typed from the keyboard")

	// The create dialog has six inputs; Tab off the last one lands on Save.
	for i := 0; i < 6; i++ {
		m = pressAndSettle(t, m, tea.KeyMsg{Type: tea.KeyTab})
	}
	if gw.hasCreateIssueCall() {
		t.Fatal("walking the fields must not create the issue")
	}

	m = pressAndSettle(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !gw.hasCreateIssueCall() {
		t.Fatalf("expected CreateIssue after submitting, calls=%#v", gw.Calls())
	}

	page, err := gw.repo.Search(t.Context(), domain.SearchIssuesQuery{Text: "Typed"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	titles := make([]string, 0, len(page.Results))
	for _, r := range page.Results {
		titles = append(titles, r.Issue.Title)
	}
	if len(titles) != 1 || titles[0] != "Typed from the keyboard" {
		t.Errorf("typed title did not reach the store: got %v", titles)
	}
}

func TestMutationDialogEscapeWritesNothing(t *testing.T) {
	t.Parallel()

	gw := newTestRepository()
	gw.seedReady("tm-1", "Ready first", "task", 1)

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	m = openDialog(t, m, 'a')
	m = typeIntoDialog(m, "discard me")
	m = pressAndSettle(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.showActionModal {
		t.Fatal("expected Escape to close the dialog")
	}
	if gw.hasAddCommentCall() {
		t.Fatalf("Escape must not write, calls=%#v", gw.Calls())
	}

	detail, err := gw.repo.Issue(t.Context(), "tm-1")
	if err != nil {
		t.Fatalf("reading tm-1 back: %v", err)
	}
	if len(detail.Comments) != 0 {
		t.Errorf("expected no comments after Escape, got %d", len(detail.Comments))
	}
}
