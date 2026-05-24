//go:build integration

package search

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/beads-workbench/internal/gateway/beads"
	repositorybeads "github.com/hk9890/beads-workbench/internal/repository/beads"
	"github.com/hk9890/beads-workbench/internal/testing/e2e/embeddedfixture"
	testui "github.com/hk9890/beads-workbench/internal/testing/ui"
)

func TestSearchModeEmbeddedFixtureInitUsesEmptyQueryFallback(t *testing.T) {
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
	repo := repositorybeads.New(runner)

	tm := testui.NewTestModelWithSize(t, testui.ControllerAdapter{Controller: NewModel(repo, nil)}, 120, 30)
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 30})

	// Real bd subprocess can take ~8s in isolation and much longer under
	// parallel `go test ./...` load — default 1s budget would flake. Bump to 15s.
	testui.WaitForOutputContainsAllWithTimeout(t, tm.Output(), 15*time.Second, "Search", "bwf-1")

	if err := tm.Quit(); err != nil {
		t.Fatalf("failed to quit teatest model: %v", err)
	}

	final, ok := tm.FinalModel(t).(testui.ControllerAdapter)
	if !ok {
		t.Fatalf("expected final model adapter")
	}

	finalModel, ok := final.Controller.(*Model)
	if !ok {
		t.Fatalf("expected wrapped search model, got %T", final.Controller)
	}

	if finalModel.errText != "" {
		t.Fatalf("expected empty-query fallback search to load without errors, got %q", finalModel.errText)
	}
	if finalModel.ResultCount() == 0 {
		t.Fatalf("expected fallback search to load fixture issues, got 0")
	}
	if strings.Contains(finalModel.View(0), "Search failed") {
		t.Fatalf("expected no runtime search failure in view, got:\n%s", finalModel.View(0))
	}
}

func hasExecutable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
