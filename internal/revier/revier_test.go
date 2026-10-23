package revier

import (
	"reflect"
	"strings"
	"testing"
)

// recordedStatus is revier status output for a project at /home/ada/demo.
const recordedStatus = `project   demo
path      /home/ada/demo
runtime   kitty
window    kitty
attached  1 window
`

// recordedList is revier list --json --conversations output with a local
// agent in each state, one whose harness names no session, and one on a
// link's remote host, which has no ref.
const recordedList = `[
  {
    "project": {"name": "demo", "path": "/home/ada/demo"},
    "running": true,
    "path_exists": true,
    "home": {"host": "kitty", "id": "12"},
    "targets": [{"name": "home", "host": "kitty", "ref": {"host": "kitty", "id": "12"}, "available": true}],
    "agents": [
      {"panel": "7", "ref": {"host": "kitty", "id": "12"}, "state": {"harness": "claude", "status": "running", "activity": "Editing model.go", "session": "s-7"}},
      {"panel": "9", "ref": {"host": "kitty", "id": "12"}, "state": {"harness": "claude", "status": "attention", "session": "s-9"}},
      {"panel": "11", "ref": {"host": "kitty", "id": "12"}, "state": {"harness": "shell", "status": "idle"}},
      {"panel": "3", "state": {"harness": "claude", "status": "running", "session": "s-remote"}}
    ]
  }
]
`

func TestProjectFromStatusReadsTheProjectAtTheRoot(t *testing.T) {
	for _, root := range []string{"/home/ada/demo", "/home/ada/demo/"} {
		got, err := projectFromStatus([]byte(recordedStatus), root)
		if err != nil {
			t.Fatalf("root %q: %v", root, err)
		}
		if got != "demo" {
			t.Errorf("root %q: project = %q, want demo", root, got)
		}
	}
}

func TestProjectFromStatusRefusesTheFocusedWindowsProject(t *testing.T) {
	_, err := projectFromStatus([]byte(recordedStatus), "/home/ada/other")
	if err == nil || !strings.Contains(err.Error(), "not a revier project") {
		t.Fatalf("expected a not-a-revier-project error, got %v", err)
	}
}

func TestProjectFromStatusRefusesOutputWithoutAProject(t *testing.T) {
	if _, err := projectFromStatus([]byte("runtime   kitty\n"), "/home/ada/demo"); err == nil {
		t.Fatal("expected an error for status output without project and path")
	}
}

func TestParseAgentsReadsTheLocalAgents(t *testing.T) {
	got, err := parseAgents([]byte(recordedList))
	if err != nil {
		t.Fatal(err)
	}
	want := []Agent{
		{Panel: "7", Status: "running", Session: "s-7", Activity: "Editing model.go"},
		{Panel: "9", Status: "attention", Session: "s-9"},
		{Panel: "11", Status: "idle"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agents = %+v\nwant %+v", got, want)
	}
}

func TestParseAgentsOfAProjectWithoutAgents(t *testing.T) {
	got, err := parseAgents([]byte(`[{"project": {"name": "demo"}, "running": false, "targets": []}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no agents, got %+v", got)
	}
}

func TestParseAgentsRefusesOutputThatIsNotTheList(t *testing.T) {
	if _, err := parseAgents([]byte("no projects.")); err == nil {
		t.Fatal("expected an error for output that is not JSON")
	}
}

func TestAddressJoinsProjectAndPanel(t *testing.T) {
	if got := Address("demo", "7"); got != "demo:7" {
		t.Fatalf("Address = %q, want demo:7", got)
	}
}
