package toaster

import (
	"strings"
	"testing"
)

func TestShowHide(t *testing.T) {
	m := New()
	if m.Visible() {
		t.Fatalf("new toaster should not be visible")
	}

	m = m.Show("done", StyleSuccess)
	if !m.Visible() {
		t.Fatalf("toaster should be visible after Show")
	}
	if !strings.Contains(m.View(), "done") {
		t.Fatalf("toast view should contain message")
	}

	m = m.Hide()
	if m.Visible() {
		t.Fatalf("toaster should not be visible after Hide")
	}
}

func TestOverlayReturnsBackgroundWhenHidden(t *testing.T) {
	bg := "background"
	m := New()
	if got := m.Overlay(bg, 20, 5); got != bg {
		t.Fatalf("expected background unchanged when hidden")
	}
}
