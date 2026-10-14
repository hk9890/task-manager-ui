package app

import (
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/config"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

func TestModelBuiltInLauncherHotkeysUseLauncherService(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1, func(i *memoryrepo.Issue) { i.Labels = []string{"ui"} })
	seedInProgress(gw, "tm-2", "In progress", "task", 2)

	fakeLauncher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gw, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, cmd := m.Update(testui.Press("enter"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	next, cmd = m.Update(testui.Press("alt+v"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if len(fakeLauncher.Calls()) != 1 || fakeLauncher.Calls()[0].Action != "nvim" {
		t.Fatalf("expected nvim launcher call before toast assertion, got %#v", fakeLauncher.Calls())
	}

	next, _ = m.Update(launchActionResultMsg{action: "nvim", err: nil})
	m = next.(Model)
	next, cmd = m.Update(testui.Press("alt+p"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))
	next, cmd = m.Update(testui.Press("alt+l"))
	m = next.(Model)
	m = applyMessages(t, m, runBatch(cmd))

	if len(fakeLauncher.Calls()) != 3 {
		t.Fatalf("expected 3 launcher calls, got %d", len(fakeLauncher.Calls()))
	}

	actions := []string{fakeLauncher.Calls()[0].Action, fakeLauncher.Calls()[1].Action, fakeLauncher.Calls()[2].Action}
	if actions[0] != "nvim" || actions[1] != "opencode" || actions[2] != "shell-command" {
		t.Fatalf("expected launcher actions [nvim opencode shell-command], got %#v", actions)
	}
}

func TestModelLauncherSuccessToastClarifiesBackgroundLifecycle(t *testing.T) {
	t.Parallel()

	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	seedInProgress(gw, "tm-2", "In progress", "task", 2)

	fakeLauncher := &fakes.FakeLauncher{}
	services, err := NewServicesWithLauncher(gw, config.Default(), fakeLauncher)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher returned error: %v", err)
	}

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))

	next, _ := m.Update(launchActionResultMsg{action: "nvim", err: nil})
	m = next.(Model)

	if view := testui.View(m); !strings.Contains(view, "background (no return flow)") {
		t.Fatalf("expected launcher lifecycle guidance toast, got:\n%s", view)
	}
}
