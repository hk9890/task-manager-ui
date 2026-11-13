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
// target work; with $REVIER_LIST_FAILS set it fails, and its first
// $REVIER_SESSION_LATE calls leave out the agent of target home, as revier
// does while a new tab has no session yet. `agent prompt` fails with
// $REVIER_PROMPT_FAILS set. `agent new` prints the address of demo:home:7,
// or of demo:home:8 with --resume. `agent wait` waits for the harness of a new tab
// itself, as revier does; with $REVIER_WAIT_TIMES_OUT set every wait exits 2,
// and with $REVIER_WAIT_FAILS set every wait exits 1. A wait until stopped
// prints $REVIER_STOPPED_ON, idle when it is not set. `agent read` prints what
// the agent said, `agent close` refuses without --force, and an unknown
// address fails as revier does: demo:home:8 is known once a harness wait
// succeeded.
const fakeRevier = `#!/bin/sh
printf '%s | %s\n' "$*" "$(pwd -P)" >> "$REVIER_LOG"
case "$1" in
status) printf 'project   demo\npath      %s\nruntime   kitty\n' "$REVIER_PROJECT_PATH" ;;
list) [ -z "$REVIER_LIST_FAILS" ] || { echo "no project demo" >&2; exit 1; }
	lists=$(( $(cat "$REVIER_LOG.lists" 2>/dev/null || echo 0) + 1 ))
	echo "$lists" > "$REVIER_LOG.lists"
	if [ "$lists" -le "${REVIER_SESSION_LATE:-0}" ]; then
		printf '[{"project":{"name":"demo"},"agents":[{"panel":"7","target":"work","ref":{"host":"kitty","id":"2"},"state":{"status":"running","session":"s-work"}}]}]\n'
		exit 0
	fi
	printf '[{"project":{"name":"demo"},"agents":[{"panel":"7","target":"work","ref":{"host":"kitty","id":"2"},"state":{"status":"running","session":"s-work"}},{"panel":"7","target":"home","ref":{"host":"kitty","id":"1"},"state":{"status":"idle","session":"s-7"}}]}]\n' ;;
agent)
	case "$2" in
	new)
		case "$*" in
		*--resume*) echo demo:home:8 ;;
		*) echo demo:home:7 ;;
		esac ;;
	wait)
		[ -z "$REVIER_WAIT_TIMES_OUT" ] || exit 2
		[ -z "$REVIER_WAIT_FAILS" ] || { echo "no agent at $3" >&2; exit 1; }
		[ "$5" != idle ] || : > "$REVIER_LOG.harness"
		[ "$5" != stopped ] || { echo "${REVIER_STOPPED_ON:-idle}"; exit 0; }
		echo idle ;;
	*)
		[ "$3" = "demo:home:7" ] || { [ "$3" = "demo:home:8" ] && [ -e "$REVIER_LOG.harness" ]; } || { echo "no agent at $3" >&2; exit 1; }
		case "$2" in
		prompt) [ -z "$REVIER_PROMPT_FAILS" ] || { echo "the prompt was refused" >&2; exit 1; } ;;
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
	workTab7 = agent.Agent{Ref: "demo:work:7", Session: "s-work", Label: "Tab work/7", Status: agent.Running}
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

// A failed list forgets the project of the root, so the next call resolves
// it again: the project may have been renamed or removed in revier.
func TestAFailedListResolvesTheProjectAgain(t *testing.T) {
	root := t.TempDir()
	h, log := withFakeRevier(t, root)
	t.Setenv("REVIER_LIST_FAILS", "1")

	for range 2 {
		if _, err := h.List(context.Background(), root); err == nil {
			t.Fatal("expected the failed list to fail")
		}
	}
	want := []string{"status", "list --json --conversations demo", "status", "list --json --conversations demo"}
	if calls := readCalls(t, log); !reflect.DeepEqual(calls, want) {
		t.Errorf("revier calls = %q, want %q", calls, want)
	}
}

func TestListRefusesARootRevierResolvesElsewhere(t *testing.T) {
	h, _ := withFakeRevier(t, "/home/someone/other-project")

	_, err := h.List(context.Background(), t.TempDir())
	if !errors.Is(err, agent.ErrNoProject) {
		t.Fatalf("expected an ErrNoProject error when revier status resolves another project, got %v", err)
	}
}

// A revier that is not installed is a fault of the host, not a root that is
// no revier project: the operator reads it as such.
func TestListSaysRevierIsNotInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := NewHost().List(context.Background(), t.TempDir())
	if err == nil || errors.Is(err, agent.ErrNoProject) || !strings.Contains(err.Error(), "revier is not installed") {
		t.Fatalf("expected a not-installed error that is no ErrNoProject, got %v", err)
	}
}

// A revier status that fails is no ErrNoProject either: it did not say
// which project the root is.
func TestAFailedStatusIsNoErrNoProject(t *testing.T) {
	root := t.TempDir()
	h, _ := withFakeRevier(t, root)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "revier"), []byte("#!/bin/sh\necho 'no server' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := h.List(context.Background(), root)
	if err == nil || errors.Is(err, agent.ErrNoProject) || !strings.Contains(err.Error(), "no server") {
		t.Fatalf("expected revier's status error, and no ErrNoProject, got %v", err)
	}
}

// Start opens the tab without the focus, waits once for its harness, reads
// the session from the list, then prompts it. It never focuses it.
func TestStartPromptsTheNewAgentOnceItsSessionIsKnown(t *testing.T) {
	root := t.TempDir()
	h, log := withFakeRevier(t, root)

	started, err := h.Start(context.Background(), agent.Spec{Root: root, Prompt: "talk about tm-1"})
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
		"list --json --conversations demo",
		"agent prompt demo:home:7 -- talk about tm-1",
	}
	if calls := readCalls(t, log); !reflect.DeepEqual(calls, want) {
		t.Errorf("revier calls = %q\nwant %q", calls, want)
	}
}

func TestStartWithoutPromptOnlyOpensTheAgent(t *testing.T) {
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
			t.Errorf("Start without a prompt ran %q", call)
		}
	}
}

// The harness wait runs once, whether it ran out of time or failed: revier
// agent wait already waits for the harness to start, so no retry helps.
func TestStartFailsOnAHarnessWaitThatFails(t *testing.T) {
	for _, env := range []string{"REVIER_WAIT_TIMES_OUT", "REVIER_WAIT_FAILS"} {
		t.Run(env, func(t *testing.T) {
			root := t.TempDir()
			h, log := withFakeRevier(t, root)
			t.Setenv(env, "1")

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
					t.Errorf("a Start whose wait failed prompted: %q", call)
				}
			}
			if waits != 1 {
				t.Errorf("a failed wait ran %d waits, want 1", waits)
			}
			// The tab is closed again: no label links it, so nothing could reach it.
			if last := calls[len(calls)-1]; last != "agent close demo:home:7 --force" {
				t.Errorf("last revier call = %q, want the force close of the new tab", last)
			}
		})
	}
}

// listCalls counts the revier list calls in calls.
func listCalls(calls []string) int {
	lists := 0
	for _, call := range calls {
		if strings.HasPrefix(call, "list ") {
			lists++
		}
	}
	return lists
}

// revier may list a new tab before it names its session: Start lists again
// until it does.
func TestStartListsAgainUntilTheSessionIsNamed(t *testing.T) {
	root := t.TempDir()
	h, log := withFakeRevier(t, root)
	t.Setenv("REVIER_SESSION_LATE", "2")

	started, err := h.Start(context.Background(), agent.Spec{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if started != tab7 {
		t.Errorf("started = %+v, want %+v", started, tab7)
	}
	if lists := listCalls(readCalls(t, log)); lists != 3 {
		t.Errorf("revier list ran %d times, want 3", lists)
	}
}

// A tab whose session revier never names is closed again: no label could
// link it, so nothing would reach it.
func TestStartClosesATabWhoseSessionIsNeverNamed(t *testing.T) {
	root := t.TempDir()
	h, log := withFakeRevier(t, root)
	t.Setenv("REVIER_SESSION_LATE", "99")

	_, err := h.Start(context.Background(), agent.Spec{Root: root, Prompt: "hi"})
	if err == nil || !strings.Contains(err.Error(), "revier lists no session for the agent at demo:home:7") {
		t.Fatalf("expected the no-session error, got %v", err)
	}
	calls := readCalls(t, log)
	if lists := listCalls(calls); lists != sessionAttempts {
		t.Errorf("revier list ran %d times, want %d", lists, sessionAttempts)
	}
	if last := calls[len(calls)-1]; last != "agent close demo:home:7 --force" {
		t.Errorf("last revier call = %q, want the force close of the new tab", last)
	}
}

// A prompt that fails returns the agent Start opened with the error, and
// leaves its tab open: the caller links it, and it may work on the issue.
func TestAFailedPromptReturnsTheOpenedAgent(t *testing.T) {
	root := t.TempDir()
	h, log := withFakeRevier(t, root)
	t.Setenv("REVIER_PROMPT_FAILS", "1")

	started, err := h.Start(context.Background(), agent.Spec{Root: root, Prompt: "hi"})
	if err == nil || !strings.Contains(err.Error(), "the prompt was refused") {
		t.Fatalf("expected the refused prompt, got %v", err)
	}
	want := agent.Agent{Ref: "demo:home:7", Session: "s-7", Label: "Tab 7", Status: agent.Idle}
	if started != want {
		t.Errorf("started = %+v, want %+v", started, want)
	}
	for _, call := range readCalls(t, log) {
		if strings.HasPrefix(call, "agent close") {
			t.Errorf("Start closed the opened agent: %q", call)
		}
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
	}
	if calls := readCalls(t, log); !reflect.DeepEqual(calls, want) {
		t.Errorf("revier calls = %q\nwant %q", calls, want)
	}
	// revier focuses a panel only once its harness has started.
	if err := h.Focus(context.Background(), resumed); err != nil {
		t.Errorf("focus after Resume = %v, want nil", err)
	}
}

// The harness of a resumed tab that does not come up closes the tab again,
// as Start does.
func TestResumeClosesATabWhoseHarnessDoesNotComeUp(t *testing.T) {
	root := t.TempDir()
	h, log := withFakeRevier(t, root)
	t.Setenv("REVIER_WAIT_TIMES_OUT", "1")

	if _, err := h.Resume(context.Background(), root, "s-8"); err == nil {
		t.Fatal("expected the resume to fail")
	}
	calls := readCalls(t, log)
	if last := calls[len(calls)-1]; last != "agent close demo:home:8 --force" {
		t.Errorf("last revier call = %q, want the force close of the new tab", last)
	}
}

func TestWaitReportsTheStatusTheAgentStoppedOn(t *testing.T) {
	h, log := withFakeRevier(t, "")

	for revierStatus, want := range map[string]agent.Status{"idle": agent.Idle, "attention": agent.NeedsYou} {
		t.Setenv("REVIER_STOPPED_ON", revierStatus)
		got, err := h.Wait(context.Background(), tab7)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("Wait on %s = %q, want %q", revierStatus, got, want)
		}
	}
	for _, call := range readCalls(t, log) {
		if call != "agent wait demo:home:7 --until stopped --timeout 1800" {
			t.Errorf("revier call = %q", call)
		}
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
