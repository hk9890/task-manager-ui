package app

// Launchers when the active store's project path is gone: refused with the
// reason, flagged in the footer, and re-enabled by a store whose path exists.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/launcher"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/storecatalog"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

// goneDir is a project path that existed and then was removed, the way a
// moved project leaves its registry entry behind.
func goneDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "moved-away")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	return dir
}

// launcherModel is an app on a store whose project is projectRoot, with a
// process runner that records instead of executing, already in Detail on the
// store's one issue.
func launcherModel(t *testing.T, cfg config.Model, projectRoot string, catalog storecatalog.Catalog) (Model, *fakes.FakeProcessRunner) {
	t.Helper()

	repo := fakes.NewTracked()
	seedReady(repo, "tm-1", "Launch target", "task", 1)

	runner := &fakes.FakeProcessRunner{}
	launcherService, err := launcher.NewService(LauncherDefinitions(cfg), projectRoot, runner)
	if err != nil {
		t.Fatalf("launcher.NewService: %v", err)
	}
	services, err := NewServicesWithLauncher(repo, cfg, launcherService)
	if err != nil {
		t.Fatalf("NewServicesWithLauncher: %v", err)
	}
	services.ProjectRoot = projectRoot
	services.ProcessRunner = runner
	services.StoreCatalog = catalog

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m = press(t, m, "3")
	if m.active != mode.Detail {
		t.Fatalf("fixture: expected Detail, got %q", m.active)
	}
	return m, runner
}

func TestLaunchersAreRefusedWhenTheProjectPathIsGone(t *testing.T) {
	gone := goneDir(t)
	m, runner := launcherModel(t, config.Default(), gone, nil)

	for _, key := range []string{"n", "p", "l"} {
		after := press(t, m, key)
		if !after.toast.Visible() || !strings.Contains(after.toast.View(), "not accessible") {
			t.Errorf("%q: expected a toast saying the project path is not accessible, got %q", key, after.toast.View())
		}
	}
	if calls := runner.Calls(); len(calls) != 0 {
		t.Errorf("a launcher ran in a project path that is gone: %+v", calls)
	}
}

// The footer says so before a key is pressed, on the surface where the launch
// keys work.
func TestTheFooterFlagsLaunchersOff(t *testing.T) {
	gone := goneDir(t)
	m, _ := launcherModel(t, config.Default(), gone, nil)

	if !strings.Contains(pickerView(m), "launchers off") {
		t.Errorf("the Detail footer does not flag the launchers as off:\n%s", pickerView(m))
	}
}

func TestLaunchersRunWhenTheProjectPathExists(t *testing.T) {
	m, runner := launcherModel(t, config.Default(), t.TempDir(), nil)

	press(t, m, "l")

	if calls := runner.Calls(); len(calls) != 1 {
		t.Errorf("launcher ran %d processes, want 1", len(calls))
	}
	if strings.Contains(pickerView(m), "launchers off") {
		t.Error("the footer flags launchers off for a project path that exists")
	}
}

// A launcher with an explicit workdir of its own never runs in the project
// path, so a missing project does not stop it.
func TestALauncherWithItsOwnWorkdirStillRuns(t *testing.T) {
	cfg := config.Default()
	own := t.TempDir()
	for i := range cfg.Launcher.Definitions {
		if cfg.Launcher.Definitions[i].Action == LaunchActionShellCommand {
			cfg.Launcher.Definitions[i].WorkDir = own
		}
	}
	m, runner := launcherModel(t, cfg, goneDir(t), nil)

	press(t, m, "l")

	calls := runner.Calls()
	if len(calls) != 1 || calls[0].Dir != own {
		t.Errorf("calls: got %+v, want one run in the launcher's own workdir %q", calls, own)
	}
}

// Launchers are a property of the active store, so a switch re-decides them:
// a store whose project exists re-enables what a store whose project is gone
// turned off.
func TestSwitchingToAStoreWithAProjectReEnablesLaunchers(t *testing.T) {
	present := t.TempDir()
	bravoRepo := fakes.NewTracked()
	seedReady(bravoRepo, "brv-1", "Bravo launch target", "task", 1)
	catalog := &fakes.FakeStoreCatalog{
		Entries: []storecatalog.Entry{
			{Name: "bravo", ProjectPath: present, StorePath: "/stores/bravo", Health: storecatalog.HealthOK},
		},
		Opens: map[string]storecatalog.Opened{"bravo": {
			Repo: bravoRepo, Name: "bravo", ProjectPath: present, StorePath: "/stores/bravo",
		}},
	}
	m, runner := launcherModel(t, config.Default(), goneDir(t), catalog)

	m = press(t, m, "s", "enter", "3")
	if m.active != mode.Detail {
		t.Fatalf("expected Detail on the switched-to store, got %q", m.active)
	}
	if strings.Contains(pickerView(m), "launchers off") {
		t.Error("launchers are still flagged off after switching to a store whose project exists")
	}

	press(t, m, "l")
	calls := runner.Calls()
	if len(calls) != 1 || calls[0].Dir != present {
		t.Errorf("calls: got %+v, want one run in the switched-to project %q", calls, present)
	}
}

// The project coming back while its store stays open re-enables the launchers
// at the next press, with no switch needed, and the footer follows.
func TestRestoringTheProjectPathReEnablesLaunchers(t *testing.T) {
	gone := goneDir(t)
	m, runner := launcherModel(t, config.Default(), gone, nil)

	m = press(t, m, "l")
	if len(runner.Calls()) != 0 {
		t.Fatal("fixture: the launcher ran while the project path was gone")
	}

	if err := os.Mkdir(gone, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	m = press(t, m, "l")

	if calls := runner.Calls(); len(calls) != 1 || calls[0].Dir != gone {
		t.Errorf("calls: got %+v, want one run in the restored project %q", calls, gone)
	}
	if strings.Contains(pickerView(m), "launchers off") {
		t.Error("the footer still flags launchers off after the project came back")
	}
}
