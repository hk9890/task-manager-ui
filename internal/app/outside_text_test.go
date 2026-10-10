package app

// A path, a directory name and an error text come from outside the app. What
// the shell draws of them is cleaned at the place every such text passes; the
// value a file or store operation uses stays raw.

import (
	"strings"
	"testing"

	storepickermode "github.com/hk9890/task-manager-ui/internal/mode/storepicker"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

const hostileDir = "/home/hans/dev/x\x1b[2Jy\tz"

// TestToastCleansEachLineAndKeepsTheLineFeeds: every toast goes through
// showToast, so an error text with an escape sequence cannot reach the
// terminal from one, and a toast of two lines is still two lines.
func TestToastCleansEachLineAndKeepsTheLineFeeds(t *testing.T) {
	m := newStorelessStart(t).m
	m.showToast("Store a\tb is not usable\ncause: x\x1b[2Jy", toaster.StyleError)

	view := m.toast.View()
	if strings.Contains(view, "\x1b[2J") || strings.Contains(view, "\t") {
		t.Errorf("the toast draws a control character from its message:\n%q", view)
	}
	for _, want := range []string{"Store a b is not usable", "cause: x [2Jy"} {
		if !strings.Contains(view, want) {
			t.Errorf("the toast lost the line %q:\n%s", want, view)
		}
	}
}

// TestStoreFormDrawsTheDirectoryCleanAndCreatesInTheRawOne: the form names the
// directory and prefills from it, and the store is created where the operator
// started the app.
func TestStoreFormDrawsTheDirectoryCleanAndCreatesInTheRawOne(t *testing.T) {
	s := newStorelessStart(t)

	next, _ := s.m.Update(storepickermode.CreateMsg{Kind: storepickermode.CentralStore, Dir: hostileDir})
	m := next.(Model)

	form := m.actionModal.View()
	if strings.Contains(form, "\x1b[2J") || strings.Contains(form, "\t") {
		t.Errorf("the create-store form draws a control character from the directory:\n%q", form)
	}
	if !strings.Contains(form, "x [2Jy z") {
		t.Errorf("the create-store form does not name the directory:\n%s", form)
	}

	submit(t, m, map[string]string{"name": "widget", "prefix": "wdg"})
	creates := s.catalog.Creates()
	if len(creates) != 1 || creates[0].Dir != hostileDir {
		t.Errorf("the store was created with %+v, want one create in the raw directory %q", creates, hostileDir)
	}
}
