//go:build integration

package revier

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fakeRevier logs every call and the physical directory it ran in to
// $REVIER_LOG. `status` resolves to $REVIER_PROJECT_PATH, `list` prints one
// local agent, and `agent focus` of an unknown address fails as revier does.
const fakeRevier = `#!/bin/sh
printf '%s | %s\n' "$*" "$(pwd -P)" >> "$REVIER_LOG"
case "$1" in
status) printf 'project   demo\npath      %s\nruntime   kitty\n' "$REVIER_PROJECT_PATH" ;;
list) printf '[{"project":{"name":"demo"},"agents":[{"panel":"7","ref":{"host":"kitty","id":"1"},"state":{"status":"running","session":"s-7"}}]}]\n' ;;
agent)
	[ "$3" = "demo:7" ] || { echo "no agent at $3" >&2; exit 1; } ;;
esac
`

// withFakeRevier puts the fake revier first on PATH, with status resolving to
// resolvedPath, and returns the file the fake logs its calls to.
func withFakeRevier(t *testing.T, resolvedPath string) string {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "revier"), []byte(fakeRevier), 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("REVIER_LOG", log)
	t.Setenv("REVIER_PROJECT_PATH", resolvedPath)
	return log
}

func readCalls(t *testing.T, log string) string {
	t.Helper()
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	return string(calls)
}

func TestExecClientResolvesTheProjectInTheRoot(t *testing.T) {
	root := t.TempDir()
	log := withFakeRevier(t, root)

	project, err := NewExecClient().Project(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if project != "demo" {
		t.Errorf("project = %q, want demo", project)
	}
	// The physical directory differs from root under a TMPDIR behind a
	// symlink (/var on macOS).
	physical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if calls := readCalls(t, log); calls != "status | "+physical+"\n" {
		t.Errorf("revier calls = %q, want status in %s", calls, physical)
	}
}

func TestExecClientRefusesARootRevierResolvesElsewhere(t *testing.T) {
	withFakeRevier(t, "/home/someone/other-project")

	if _, err := NewExecClient().Project(context.Background(), t.TempDir()); err == nil {
		t.Fatal("expected an error when revier status resolves another project")
	}
}

func TestExecClientListsTheAgentsOfTheProject(t *testing.T) {
	log := withFakeRevier(t, "")

	agents, err := NewExecClient().Agents(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if want := []Agent{{Panel: "7", Status: "running", Session: "s-7"}}; !reflect.DeepEqual(agents, want) {
		t.Errorf("agents = %+v, want %+v", agents, want)
	}
	if calls := readCalls(t, log); !strings.HasPrefix(calls, "list --json --conversations demo | ") {
		t.Errorf("revier calls = %q, want list --json --conversations demo", calls)
	}
}

func TestExecClientFocusesTheAgentAtTheAddress(t *testing.T) {
	log := withFakeRevier(t, "")

	if err := NewExecClient().Focus(context.Background(), Address("demo", "7")); err != nil {
		t.Fatal(err)
	}
	if calls := readCalls(t, log); !strings.HasPrefix(calls, "agent focus demo:7 | ") {
		t.Errorf("revier calls = %q, want agent focus demo:7", calls)
	}
}

func TestExecClientReportsRevierStandardError(t *testing.T) {
	withFakeRevier(t, "")

	err := NewExecClient().Focus(context.Background(), Address("demo", "8"))
	if err == nil || !strings.Contains(err.Error(), "no agent at demo:8") {
		t.Fatalf("expected the error to carry revier's message, got %v", err)
	}
}

func TestExecClientSaysRevierIsNotInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := NewExecClient().Agents(context.Background(), "demo")
	if err == nil || !strings.Contains(err.Error(), "revier is not installed") {
		t.Fatalf("expected a not-installed error, got %v", err)
	}
}
