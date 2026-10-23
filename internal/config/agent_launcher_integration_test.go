//go:build integration

package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRevier logs every call to $REVIER_LOG. `status` resolves to
// $REVIER_PROJECT_PATH, `agent new` prints an address, and the first
// `agent wait` fails as it does before the harness in the new tab has started.
// With $REVIER_WAIT_TIMES_OUT set, every `agent wait` exits 2, as a wait that
// ran out of time does.
const fakeRevier = `#!/bin/sh
printf '%s\n' "$*" >> "$REVIER_LOG"
case "$1 $2" in
"status "*) printf 'project   demo\npath      %s\nruntime   kitty\n' "$REVIER_PROJECT_PATH" ;;
"agent new") echo demo:7 ;;
"agent wait")
	[ -z "$REVIER_WAIT_TIMES_OUT" ] || exit 2
	[ -e "$REVIER_LOG.started" ] || { : > "$REVIER_LOG.started"; exit 1; }
	echo idle ;;
esac
`

// runAgentLauncher runs the default agent launcher body in projectRoot for
// issue tm-42, with a fake revier whose status resolves to resolvedPath, and
// returns the revier calls it made. env is added to the body's environment.
func runAgentLauncher(t *testing.T, projectRoot, resolvedPath string, env ...string) (string, error) {
	t.Helper()

	var agent LauncherDefinition
	for _, d := range Default().Launcher.Definitions {
		if d.Action == "agent" {
			agent = d
		}
	}
	if agent.Command != "sh" || len(agent.Args) != 3 || agent.Args[0] != "-c" || agent.Args[2] != "{{issue.id}}" {
		t.Fatalf("expected the agent launcher to run sh -c <body> {{issue.id}}, got %#v", agent)
	}

	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "revier"), []byte(fakeRevier), 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "calls")

	cmd := exec.Command("sh", "-c", agent.Args[1], "tm-42")
	cmd.Dir = projectRoot
	// PWD is what os/exec sets beside Dir when Env is nil, as it is for this
	// launcher in the app. Without it sh takes the physical path of Dir, which
	// differs from projectRoot under a TMPDIR behind a symlink (/var on macOS).
	cmd.Env = append(os.Environ(), "PWD="+projectRoot, "PATH="+bin+":"+os.Getenv("PATH"), "REVIER_LOG="+log, "REVIER_PROJECT_PATH="+resolvedPath)
	cmd.Env = append(cmd.Env, env...)
	runErr := cmd.Run()

	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	return string(calls), runErr
}

func TestAgentLauncherPromptsTheNewAgentOnceItIsIdle(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	calls, err := runAgentLauncher(t, root, root)
	if err != nil {
		t.Fatalf("agent launcher body failed: %v\ncalls:\n%s", err, calls)
	}

	want := strings.Join([]string{
		"status",
		"agent new --no-focus -p demo",
		"agent wait demo:7 --until idle --timeout 60",
		"agent wait demo:7 --until idle --timeout 60",
		"agent prompt demo:7 -- Run this command, then stop: taskmgr comment add tm-42 hello",
	}, "\n") + "\n"
	if calls != want {
		t.Fatalf("revier calls:\n%s\nwant:\n%s", calls, want)
	}
}

func TestAgentLauncherAddsNoAgentWhenRevierResolvesAnotherProject(t *testing.T) {
	t.Parallel()

	calls, err := runAgentLauncher(t, t.TempDir(), "/home/someone/other-project")
	if err == nil {
		t.Fatal("expected the agent launcher body to fail")
	}
	if calls != "status\n" {
		t.Fatalf("expected status as the only revier call, got:\n%s", calls)
	}
}

func TestAgentLauncherStopsWhenTheWaitTimesOut(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	calls, err := runAgentLauncher(t, root, root, "REVIER_WAIT_TIMES_OUT=1")
	if err == nil {
		t.Fatal("expected the agent launcher body to fail")
	}

	want := strings.Join([]string{
		"status",
		"agent new --no-focus -p demo",
		"agent wait demo:7 --until idle --timeout 60",
	}, "\n") + "\n"
	if calls != want {
		t.Fatalf("revier calls:\n%s\nwant:\n%s", calls, want)
	}
}
