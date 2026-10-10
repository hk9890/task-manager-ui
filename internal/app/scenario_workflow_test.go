package app

import (
	"strconv"
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

func TestModelReusableBoardDetailScenarioCoversScrollAndBack(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedInProgress(gw, "tm-2", "In progress", "task", 2)
	seedIssueDetail(gw, domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1},
		Description: longScenarioDetail(90),
	})

	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}

	m := testui.InitializeModel(mustNewModel(t, services)).(Model)
	m.width, m.height = 120, 24

	m = testui.ApplyKeySequence(m, testui.OpenDetailKeys()...).(Model)
	if m.active != mode.Detail {
		t.Fatalf("expected board->detail open scenario, got %s", m.active)
	}

	m = testui.ApplyKeySequence(m, testui.DetailScrollKeys()...).(Model)
	if m.detail.ContentScrollOffset == 0 {
		t.Fatalf("expected detail scroll scenario to move viewport offset")
	}

	m = testui.ApplyKeySequence(m, testui.DetailBackKeys()...).(Model)
	if m.active != mode.Board {
		t.Fatalf("expected detail back scenario to return to board, got %s", m.active)
	}
}

func TestModelReusableDetailToolScenarioCoversEditorAndLaunchersWithFakes(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedInProgress(gw, "tm-2", "In progress", "task", 2)
	seedIssueDetail(gw, domain.IssueDetail{
		Summary:     domain.IssueSummary{ID: "tm-1", Title: "Ready first", Status: "open", Type: "task", Priority: 1},
		Description: "detail",
	})

	fakeLauncher := &fakes.FakeLauncher{}
	fakeEditor := &fakes.FakeEditor{}
	services, err := NewServicesWithLauncher(gw, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}
	services.Editor = fakeEditor

	m := testui.InitializeModel(mustNewModel(t, services)).(Model)
	m = testui.ApplyKeySequence(m, testui.OpenDetailKeys()...).(Model)
	if m.active != mode.Detail {
		t.Fatalf("expected open detail scenario before tool actions, got %s", m.active)
	}

	m = testui.ApplyKeySequence(m, testKey("alt+e")).(Model)
	if len(fakeEditor.Calls()) != 1 || fakeEditor.Calls()[0].IssueID != "tm-1" {
		t.Fatalf("expected edit seam call for tm-1, got %#v", fakeEditor.Calls())
	}

	m = testui.ApplyKeySequence(m,
		testKey("alt+v"),
		testKey("alt+p"),
		testKey("alt+l"),
	).(Model)

	if len(fakeLauncher.Calls()) != 3 {
		t.Fatalf("expected 3 launcher seam calls, got %d", len(fakeLauncher.Calls()))
	}
	actions := []string{fakeLauncher.Calls()[0].Action, fakeLauncher.Calls()[1].Action, fakeLauncher.Calls()[2].Action}
	if strings.Join(actions, ",") != "nvim,opencode,shell-command" {
		t.Fatalf("expected launcher actions [nvim opencode shell-command], got %#v", actions)
	}
}

func longScenarioDetail(lines int) string {
	out := make([]string, 0, lines)
	for i := 1; i <= lines; i++ {
		out = append(out, "Line "+strconv.Itoa(i))
	}
	return strings.Join(out, "\n")
}
