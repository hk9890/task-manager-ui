package app

// A paste arrives as one message, not as keys. It goes where the typed text
// would go, and nowhere a key is held from.

import (
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
)

func TestAPasteFiltersTheActiveTabAlone(t *testing.T) {
	m, _ := filterShell(t)

	m = send(t, m, testui.Paste("login"))
	if view := plainShell(m); !strings.Contains(view, "❯ login") || strings.Contains(view, "Triage inbox") {
		t.Fatalf("the paste did not filter the board:\n%s", view)
	}

	m = pressKey(t, m, "tab")
	if view := plainShell(m); !strings.Contains(view, "❯ filter docs") || !strings.Contains(view, "Session notes") {
		t.Fatalf("the paste on the board reached the docs query:\n%s", view)
	}
}

func TestAPastedCRLFIsOneSpaceInTheQuery(t *testing.T) {
	m, _ := filterShell(t)

	m = send(t, m, testui.Paste("log\r\nin"))
	if view := plainShell(m); !strings.Contains(view, "❯ log in ") {
		t.Fatalf("the pasted CRLF is not one space in the query:\n%s", view)
	}
}

// The form's modal schedules a repeating cursor tick, so its commands are left
// unrun (openForm).
func TestAPasteGoesToTheStoreFormsFocusedInput(t *testing.T) {
	s := newStorelessStart(t)
	m := openForm(t, s.m, 0)

	next, _ := m.Update(testui.Paste("xy"))
	m = next.(Model)
	if form := pickerView(m); !strings.Contains(form, "pfxxy") {
		t.Fatalf("the paste did not reach the prefix input:\n%s", form)
	}
}

func TestAPasteGoesToTheEditorsFocusedField(t *testing.T) {
	m, gw := editorShell(t)

	m = openEditor(t, m, "alt+n")
	m = send(t, m, testui.Paste("Pasted title"))
	for range 5 { // type, priority, assignee, labels, description
		m = pressKey(t, m, "tab")
	}
	m = send(t, m, testui.Paste("first line\nsecond line"))
	m = pressKey(t, m, "ctrl+s")

	calls := gw.CallsFor(fakes.MethodCreateIssue)
	if len(calls) != 1 {
		t.Fatalf("expected one CreateIssue call, got %d; toast %q", len(calls), m.toast.View())
	}
	if input := calls[0].Args.(domain.CreateIssueInput); input.Title != "Pasted title" || input.Description != "first line\nsecond line" {
		t.Errorf("CreateIssue received title %q and description %q", input.Title, input.Description)
	}
	if view := plainShell(m); !strings.Contains(view, "❯ filter issues") {
		t.Errorf("a paste into the editor reached the board query:\n%s", view)
	}
}

func TestAPasteUnderHelpReachesNothing(t *testing.T) {
	m, _ := filterShell(t)
	m = pressKey(t, m, "alt+h")
	whole := testui.View(m)

	next, cmd := m.Update(testui.Paste("login"))
	m = next.(Model)
	if cmd != nil || !m.showHelp || testui.View(m) != whole {
		t.Fatalf("a paste under help changed it: help %v, command %v", m.showHelp, cmd != nil)
	}

	m = pressKey(t, m, "esc")
	if view := plainShell(m); m.active != mode.Board || !strings.Contains(view, "❯ filter issues") {
		t.Fatalf("a paste under help reached the board:\n%s", view)
	}
}
