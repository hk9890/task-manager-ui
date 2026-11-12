//go:build integration

package revier

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/agent"
)

// fakeRevier logs every call and the physical directory it ran in to
// $REVIER_LOG. `status` resolves to $REVIER_PROJECT_PATH; `list` prints one
// local agent in panel 7 of target home, and first another in panel 7 of
// target work; `agent new` prints the address of demo:home:7, or of
// demo:home:8 with --resume. The first `agent wait --until idle` fails, as it
// does before the harness of a new tab has started; with
// $REVIER_WAIT_TIMES_OUT set every wait exits 2. `agent read` prints what the
// agent said, `agent close` refuses without --force, and an unknown address
// fails as revier does: demo:home:8 is known once a harness wait succeeded.
const fakeRevier = `#!/bin/sh
printf '%s | %s\n' "$*" "$(pwd -P)" >> "$REVIER_LOG"
case "$1" in
status) printf 'project   demo\npath      %s\nruntime   kitty\n' "$REVIER_PROJECT_PATH" ;;
list) printf '[{"project":{"name":"demo"},"agents":[{"panel":"7","target":"work","ref":{"host":"kitty","id":"2"},"state":{"status":"running","session":"s-work"}},{"panel":"7","target":"home","ref":{"host":"kitty","id":"1"},"state":{"status":"idle","session":"s-7"}}]}]\n' ;;
agent)
	case "$2" in
	new)
		case "$*" in
		*--resume*) echo demo:home:8 ;;
		*) echo demo:home:7 ;;
		esac ;;
	wait)
		[ -z "$REVIER_WAIT_TIMES_OUT" ] || exit 2
		if [ "$5" = idle ] && [ ! -e "$REVIER_LOG.started" ]; then
			: > "$REVIER_LOG.started"
			echo "no agent at $3" >&2
			exit 1
		fi
		[ "$5" != idle ] || : > "$REVIER_LOG.harness"
		echo idle ;;
	*)
		[ "$3" = "demo:home:7" ] || { [ "$3" = "demo:home:8" ] && [ -e "$REVIER_LOG.harness" ]; } || { echo "no agent at $3" >&2; exit 1; }
		case "$2" in
		read) echo 'all done' ;;
		close) [ "$4" = "--force" ] || { echo "the agent is running" >&2; exit 1; } ;;
		esac ;;
	esac ;;
esac
`

// withFakeRevier puts the fake revier first on PATH, with status resolving to
// resolvedPath, and returns a Host that does not pause between attempts and
// the file the fake logs its calls to.
func withFakeRevier(t *testing.T, resolvedPath string) (*Host, string) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "revier"), []byte(fakeRevier), 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "calls")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("REVIER_LOG", log)
	t.Setenv("REVIER_PROJECT_PATH", resolvedPath)
	h := NewHost()
	h.pause = 0
	return h, log
}

// readCalls returns the revier calls logged so far, without the directory
// each ran in.
func readCalls(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		call, _, _ := strings.Cut(line, " | ")
		calls = append(calls, call)
	}
	return calls
}

var (
	tab7     = agent.Agent{Ref: "demo:home:7", Session: "s-7", Label: "Tab 7", Status: agent.Idle}
	workTab7 = agent.Agent{Ref: "demo:work:7", Session: "s-work", Label: "Tab 7", Status: agent.Running}
)

// List asks revier status in the root once, and revier list on every call.
func TestListResolvesTheProjectInTheRootOnce(t *testing.T) {
	root := t.TempDir()
	h, log := withFakeRevier(t, root)

	for range 2 {
		agents, err := h.List(context.Background(), root)
		if err != nil {
			t.Fatal(err)
		}
		if want := []agent.Agent{workTab7, tab7}; !reflect.DeepEqual(agents, want) {
			t.Errorf("agents = %+v, want %+v", agents, want)
		}
	}
	want := []string{"status", "list --json --conversations demo", "list --json --conversations demo"}
	if calls := readCalls(t, log); !reflect.DeepEqual(calls, want) {
		t.Errorf("revier calls = %q, want %q", calls, want)
	}
	// The physical directory differs from root under a TMPDIR behind a
	// symlink (/var on macOS).
	physical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(log)
	if first, _, _ := strings.Cut(string(data), "\n"); first != "status | "+physical {
		t.Errorf("first call = %q, want status in %s", first, physical)
	}
}

func TestListRefusesARootRevierResolvesElsewhere(t *testing.T) {
	h, _ := withFakeRevier(t, "/home/someone/other-project")

	_, err := h.List(context.Background(), t.TempDir())
	if !errors.Is(err, agent.ErrNoProject) {
		t.Fatalf("expected an ErrNoProject error when revier status resolves another project, got %v", err)
	}
}

func TestListSaysRevierIsNotInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := NewHost().List(context.Background(), t.TempDir())
	if !errors.Is(err, agent.ErrNoProject) || !strings.Contains(err.Error(), "revier is not installed") {
		t.Fatalf("expected a not-installed ErrNoProject error, got %v", err)
	}
}

// Start opens the tab without the focus, waits again while the harness has
// not started, reads the session from the list, then prompts and focuses.
func TestStartPromptsAndFocusesTheNewAgentOnceItsSessionIsKnown(t *testing.T) {
	root := t.TempDir()
	h, log := withFakeRevier(t, root)

	started, err := h.Start(context.Background(), agent.Spec{Root: root, Prompt: "talk about tm-1", Focus: true})
	if err != nil {
		t.Fatal(err)
	}
	if want := (agent.Agent{Ref: "demo:home:7", Session: "s-7", Label: "Tab 7", Status: agent.Running}); started != want {
		t.Errorf("started = %+v, want %+v", started, want)
	}
	want := []string{
		"status",
		"agent new --no-focus -p demo",
		"agent wait demo:home:7 --until idle --timeout 60",
		"agent wait demo:home:7 --until idle --timeout 60",
		"list --json --conversations demo",
		"agent prompt demo:home:7 -- talk about tm-1",
		"agent focus demo:home:7",
	}
	if calls := readCalls(t, log); !reflect.DeepEqual(calls, want) {
		t.Errorf("revier calls = %q\nwant %q", calls, want)
	}
}

func TestStartWithoutPromptOrFocusOnlyOpensTheAgent(t *testing.T) {
	root := t.TempDir()
	h, log := withFakeRevier(t, root)

	started, err := h.Start(context.Background(), agent.Spec{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if started != tab7 {
		t.Errorf("started = %+v, want %+v", started, tab7)
	}
	for _, call := range readCalls(t, log) {
		if strings.HasPrefix(call, "agent prompt") || strings.HasPrefix(call, "agent focus") {
			t.Errorf("Start without prompt and focus ran %q", call)
		}
	}
}

// A wait that ran out of time is not tried again: the agent is there and not
// idle, which no retry changes.
func TestStartFailsOnAHarnessWaitThatTimesOut(t *testing.T) {
	root := t.TempDir()
	h, log := withFakeRevier(t, root)
	t.Setenv("REVIER_WAIT_TIMES_OUT", "1")

	if _, err := h.Start(context.Background(), agent.Spec{Root: root, Prompt: "hi"}); err == nil || !strings.Contains(err.Error(), "not idle") {
		t.Fatalf("expected a not-idle error, got %v", err)
	}
	waits := 0
	calls := readCalls(t, log)
	for _, call := range calls {
		if strings.HasPrefix(call, "agent wait") {
			waits++
		}
		if strings.HasPrefix(call, "agent prompt") {
			t.Errorf("a Start whose wait timed out prompted: %q", call)
		}
	}
	if waits != 1 {
		t.Errorf("a timed-out wait ran %d waits, want 1", waits)
	}
	// The tab is closed again: no label links it, so nothing could reach it.
	if last := calls[len(calls)-1]; last != "agent close demo:home:7 --force" {
		t.Errorf("last revier call = %q, want the force close of the new tab", last)
	}
}

func TestResumeOpensTheConversationWithoutTheFocus(t *testing.T) {
	root := t.TempDir()
	h, log := withFakeRevier(t, root)

	resumed, err := h.Resume(context.Background(), root, "s-8")
	if err != nil {
		t.Fatal(err)
	}
	if want := (agent.Agent{Ref: "demo:home:8", Session: "s-8", Label: "Tab 8", Status: agent.Idle}); resumed != want {
		t.Errorf("resumed = %+v, want %+v", resumed, want)
	}
	want := []string{
		"status",
		"agent new --no-focus -p demo --resume s-8",
		"agent wait demo:home:8 --until idle --timeout 60",
		"agent wait demo:home:8 --until idle --timeout 60",
	}
	if calls := readCalls(t, log); !reflect.DeepEqual(calls, want) {
		t.Errorf("revier calls = %q\nwant %q", calls, want)
	}
	// revier focuses a panel only once its harness has started.
	if err := h.Focus(context.Background(), resumed); err != nil {
		t.Errorf("focus after Resume = %v, want nil", err)
	}
}

func TestWaitWaitsUntilTheAgentStopped(t *testing.T) {
	h, log := withFakeRevier(t, "")

	if err := h.Wait(context.Background(), tab7); err != nil {
		t.Fatal(err)
	}
	if calls := readCalls(t, log); !reflect.DeepEqual(calls, []string{"agent wait demo:home:7 --until stopped --timeout 1800"}) {
		t.Errorf("revier calls = %q", calls)
	}
}

func TestReadReturnsWhatTheAgentSaid(t *testing.T) {
	h, _ := withFakeRevier(t, "")

	said, err := h.Read(context.Background(), tab7)
	if err != nil {
		t.Fatal(err)
	}
	if said != "all done\n" {
		t.Errorf("Read = %q, want all done", said)
	}
}

func TestFocusAndCloseActOnTheAgentsAddress(t *testing.T) {
	h, log := withFakeRevier(t, "")
	ctx := context.Background()

	if err := h.Focus(ctx, tab7); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(ctx, tab7, false); err == nil || !strings.Contains(err.Error(), "the agent is running") {
		t.Errorf("Close without force = %v, want revier's refusal", err)
	}
	if err := h.Close(ctx, tab7, true); err != nil {
		t.Fatal(err)
	}
	want := []string{"agent focus demo:home:7", "agent close demo:home:7", "agent close demo:home:7 --force"}
	if calls := readCalls(t, log); !reflect.DeepEqual(calls, want) {
		t.Errorf("revier calls = %q, want %q", calls, want)
	}
}

func TestACallReportsRevierStandardError(t *testing.T) {
	h, _ := withFakeRevier(t, "")

	err := h.Focus(context.Background(), agent.Agent{Ref: "demo:home:8"})
	if err == nil || !strings.Contains(err.Error(), "no agent at demo:home:8") {
		t.Fatalf("expected the error to carry revier's message, got %v", err)
	}
}
