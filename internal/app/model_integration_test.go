//go:build integration

package app

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/gateway/beads"
	"github.com/hk9890/beads-workbench/internal/mode"
	"github.com/hk9890/beads-workbench/internal/testing/e2e/embeddedfixture"
	"github.com/hk9890/beads-workbench/internal/testing/fakes"
	"github.com/hk9890/beads-workbench/internal/testing/ui"
)

func TestModelEmbeddedFixtureBoardToDetailSmokeWorkflow(t *testing.T) {
	if !hasExecutable("bd") || !hasExecutable("jq") || !hasExecutable("git") {
		t.Skip("requires bd, jq, and git on PATH")
	}
	t.Setenv("BEADS_ACTOR", "fixture-user")

	repoPath := embeddedfixture.TempRepoPath(t)
	embeddedfixture.Seed(t, repoPath)

	runner := beads.NewCommandRunner(beads.RunnerConfig{
		WorkDir: repoPath,
		Env:     append(os.Environ(), "BD_NON_INTERACTIVE=1"),
	})
	gateway := beads.NewCLIGateway(runner)

	services, err := NewServices(gateway, config.Default(), repoPath)
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	// Startup lands on Not Ready lane when blocked work exists.
	if got := firstSelectionID(m, mode.Board); got != "bwf-2" {
		t.Fatalf("expected startup board selection bwf-2 from Not Ready lane, got %q", got)
	}

	if m.detail.Detail.Summary.ID != "bwf-2" {
		t.Fatalf("expected shell detail cache to load bwf-2, got %q", m.detail.Detail.Summary.ID)
	}

	if view := m.View(); strings.Contains(view, "Selected Issue") {
		t.Fatalf("expected no sidebar on browse board, got:\n%s", view)
	}

	// Open dedicated detail mode from board selection.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Detail {
		t.Fatalf("expected active mode detail after enter, got %s", m.active)
	}
	if m.detail.TargetID != "bwf-2" {
		t.Fatalf("expected detail target bwf-2 in dedicated mode, got %q", m.detail.TargetID)
	}

	view := m.View()
	if !strings.Contains(view, "Blocked bug for fixture") {
		t.Fatalf("expected dedicated detail rendering for fixture issue, got:\n%s", view)
	}
	if strings.Contains(view, "Issue Detail") {
		t.Fatalf("expected detail mode to avoid extra shell wrapper heading, got:\n%s", view)
	}
	if !strings.Contains(view, "Assignee: bob") {
		t.Fatalf("expected detail metadata to show fixture assignee bob, got:\n%s", view)
	}
	if strings.Contains(view, "Assignee: hans.kohlreiter@dynatrace.com") {
		t.Fatalf("expected detail metadata to avoid owner in assignee slot, got:\n%s", view)
	}
}

func TestModelEmbeddedFixtureDetailEditHotkeyUsesEditorService(t *testing.T) {
	if !hasExecutable("bd") || !hasExecutable("jq") || !hasExecutable("git") {
		t.Skip("requires bd, jq, and git on PATH")
	}
	t.Setenv("BEADS_ACTOR", "fixture-user")

	repoPath := embeddedfixture.TempRepoPath(t)
	embeddedfixture.Seed(t, repoPath)

	runner := beads.NewCommandRunner(beads.RunnerConfig{
		WorkDir: repoPath,
		Env:     append(os.Environ(), "BD_NON_INTERACTIVE=1"),
	})
	gateway := beads.NewCLIGateway(runner)

	fakeLauncher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gateway, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}
	fakeEditor := &fakes.FakeEditor{}
	services.Editor = fakeEditor

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	if got := firstSelectionID(m, mode.Board); got != "bwf-2" {
		t.Fatalf("expected startup board selection bwf-2 from fixture seed, got %q", got)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Detail {
		t.Fatalf("expected detail mode after enter, got %s", m.active)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected edit command from detail edit hotkey")
	}

	next, _ = m.Update(cmd())
	m = next.(Model)

	if len(fakeEditor.Calls) != 1 {
		t.Fatalf("expected one editor call, got %d", len(fakeEditor.Calls))
	}
	if fakeEditor.Calls[0].IssueID != "bwf-2" {
		t.Fatalf("expected editor issue bwf-2 from embedded fixture, got %q", fakeEditor.Calls[0].IssueID)
	}
	if len(fakeLauncher.Calls) != 0 {
		t.Fatalf("expected no launcher call from detail edit hotkey, got %#v", fakeLauncher.Calls)
	}
}

func TestModelEmbeddedFixtureMutationModalsOpenWithoutCatalogDecodeToast(t *testing.T) {
	if !hasExecutable("bd") || !hasExecutable("jq") || !hasExecutable("git") {
		t.Skip("requires bd, jq, and git on PATH")
	}
	t.Setenv("BEADS_ACTOR", "fixture-user")

	repoPath := embeddedfixture.TempRepoPath(t)
	embeddedfixture.Seed(t, repoPath)

	runner := beads.NewCommandRunner(beads.RunnerConfig{
		WorkDir: repoPath,
		Env:     append(os.Environ(), "BD_NON_INTERACTIVE=1"),
	})
	gateway := beads.NewCLIGateway(runner)

	services, err := NewServices(gateway, config.Default(), repoPath)
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

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected create flow command")
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	assertModalOpenWithoutCatalogToast(m, "Create Issue")

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(Model)
	}
	if m.showActionModal {
		t.Fatal("expected create modal to close on escape")
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected update flow command")
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	assertModalOpenWithoutCatalogToast(m, "Update Issue bwf-2")

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(Model)
	}
	if m.showActionModal {
		t.Fatal("expected update modal to close on escape")
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	if m.active != mode.Detail {
		t.Fatalf("expected detail mode before comment flow, got %s", m.active)
	}

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected comment flow command")
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	assertModalOpenWithoutCatalogToast(m, "Comment on bwf-2")
}

func TestModelEmbeddedFixtureFullBoardCaptureGolden(t *testing.T) {
	if !hasExecutable("bd") || !hasExecutable("jq") || !hasExecutable("git") {
		t.Skip("requires bd, jq, and git on PATH")
	}
	t.Setenv("BEADS_ACTOR", "fixture-user")

	repoPath := embeddedfixture.TempRepoPath(t)
	embeddedfixture.Seed(t, repoPath)

	runner := beads.NewCommandRunner(beads.RunnerConfig{
		WorkDir: repoPath,
		Env:     append(os.Environ(), "BD_NON_INTERACTIVE=1"),
	})
	gateway := beads.NewCLIGateway(runner)

	services, err := NewServices(gateway, config.Default(), repoPath)
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.width = 120
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	view := m.View()
	if strings.Contains(view, "Selected Issue") {
		t.Fatalf("expected board view without selected issue sidebar, got:\n%s", view)
	}
	if !strings.Contains(view, "bwf-1 Seed fixture roo") {
		t.Fatalf("expected realistic fixture issue title in board capture, got:\n%s", view)
	}
	if strings.Count(view, "│") < 20 {
		t.Fatalf("expected full-height board lanes rather than floating boxes, got:\n%s", view)
	}

	ui.AssertMatchesGoldenNormalized(t, []byte(view), "model_embedded_board_w120.golden")
}

func TestModelEmbeddedFixtureStartupLoadsBoardWithoutGatewaySectionErrors(t *testing.T) {
	if !hasExecutable("bd") || !hasExecutable("jq") || !hasExecutable("git") {
		t.Skip("requires bd, jq, and git on PATH")
	}
	t.Setenv("BEADS_ACTOR", "fixture-user")

	repoPath := embeddedfixture.TempRepoPath(t)
	embeddedfixture.Seed(t, repoPath)

	runner := beads.NewCommandRunner(beads.RunnerConfig{
		WorkDir: repoPath,
		Env:     append(os.Environ(), "BD_NON_INTERACTIVE=1"),
	})
	gateway := beads.NewCLIGateway(runner)

	services, err := NewServices(gateway, config.Default(), repoPath)
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m.width = 120
	m.height = 34
	m = applyMessages(t, m, runBatch(m.Init()))

	view := m.View()
	ui.AssertStartupBoardLayoutSanity(t, view)
	ui.AssertContainsAll(t, view, "bwf-1")
	ui.AssertNoObviousRuntimeErrorPanels(t, view)
}

func TestModelEmbeddedFixtureDetailShowsRelatedFromRealBDRelatedLink(t *testing.T) {
	if !hasExecutable("bd") || !hasExecutable("jq") || !hasExecutable("git") {
		t.Skip("requires bd, jq, and git on PATH")
	}
	t.Setenv("BEADS_ACTOR", "fixture-user")

	repoPath := embeddedfixture.TempRepoPath(t)
	embeddedfixture.Seed(t, repoPath)

	if err := runBDInRepo(repoPath, "link", "bwf-2", "bwf-3", "--type", "related"); err != nil {
		t.Fatalf("failed to create real related link: %v", err)
	}

	runner := beads.NewCommandRunner(beads.RunnerConfig{
		WorkDir: repoPath,
		Env:     append(os.Environ(), "BD_NON_INTERACTIVE=1"),
	})
	gateway := beads.NewCLIGateway(runner)

	services, err := NewServices(gateway, config.Default(), repoPath)
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	if got := firstSelectionID(m, mode.Board); got != "bwf-2" {
		t.Fatalf("expected startup board selection bwf-2 from Not Ready lane, got %q", got)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Detail {
		t.Fatalf("expected active mode detail after enter, got %s", m.active)
	}

	view := m.View()
	if !strings.Contains(view, "Related") {
		t.Fatalf("expected related rail/section in detail view, got:\n%s", view)
	}
	if !strings.Contains(view, "bwf-3") {
		t.Fatalf("expected linked related issue bwf-3 in detail view, got:\n%s", view)
	}
	if !strings.Contains(view, "bwf-3") {
		t.Fatalf("expected related issue id in detail view, got:\n%s", view)
	}
}

func TestModelEmbeddedFixtureDetailShowsRelatesToDependentOnlyUnderRelated(t *testing.T) {
	if !hasExecutable("bd") || !hasExecutable("jq") || !hasExecutable("git") {
		t.Skip("requires bd, jq, and git on PATH")
	}
	t.Setenv("BEADS_ACTOR", "fixture-user")

	repoPath := embeddedfixture.TempRepoPath(t)
	embeddedfixture.Seed(t, repoPath)

	if err := runBDInRepo(repoPath, "dep", "relate", "bwf-3", "bwf-2"); err != nil {
		t.Fatalf("failed to create real relates-to dependency: %v", err)
	}

	runner := beads.NewCommandRunner(beads.RunnerConfig{
		WorkDir: repoPath,
		Env:     append(os.Environ(), "BD_NON_INTERACTIVE=1"),
	})
	gateway := beads.NewCLIGateway(runner)

	services, err := NewServices(gateway, config.Default(), repoPath)
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	if got := firstSelectionID(m, mode.Board); got != "bwf-2" {
		t.Fatalf("expected startup board selection bwf-2 from Not Ready lane, got %q", got)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if m.active != mode.Detail {
		t.Fatalf("expected active mode detail after enter, got %s", m.active)
	}

	view := m.View()
	if !strings.Contains(view, "Blocks (0)") {
		t.Fatalf("expected relates-to dependent not to appear under blocks, got:\n%s", view)
	}
	if !strings.Contains(view, "Related (1)") {
		t.Fatalf("expected exactly one related entry from relates-to dependent, got:\n%s", view)
	}
	if strings.Count(view, "bwf-3") != 1 {
		t.Fatalf("expected relates-to-linked issue bwf-3 to render once (under Related only), got:\n%s", view)
	}
}

func hasExecutable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func runBDInRepo(repoPath string, args ...string) error {
	cmd := exec.Command("bd", args...)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd %s failed: %w\n%s", strings.Join(args, " "), err, out)
	}

	return nil
}
