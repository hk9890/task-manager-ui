package ui

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

const defaultWidth = 80
const defaultHeight = 24

// NewTestModel creates a Bubble Tea test program with deterministic terminal size.
func NewTestModel(tb testing.TB, model tea.Model) *teatest.TestModel {
	tb.Helper()

	return teatest.NewTestModel(tb, model, teatest.WithInitialTermSize(defaultWidth, defaultHeight))
}

// NewTestModelWithSize creates a Bubble Tea test program at an explicit size.
func NewTestModelWithSize(tb testing.TB, model tea.Model, width, height int) *teatest.TestModel {
	tb.Helper()

	if width <= 0 {
		width = defaultWidth
	}
	if height <= 0 {
		height = defaultHeight
	}

	return teatest.NewTestModel(tb, model, teatest.WithInitialTermSize(width, height))
}

// ReadGolden returns testdata golden bytes for the given package-local test.
func ReadGolden(tb testing.TB, name string) []byte {
	tb.Helper()

	path := filepath.Join("testdata", name)
	bts, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("failed to read golden file %q: %v", path, err)
	}

	return bts
}

// AssertMatchesGolden compares output against a package-local golden file.
// It strips \r from the golden file before comparison to handle Windows
// CRLF line endings that may be present in existing checkouts.
func AssertMatchesGolden(tb testing.TB, output []byte, name string) {
	tb.Helper()

	want := ReadGolden(tb, name)
	normalizedWant := bytes.TrimSuffix(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), []byte("\n"))
	normalizedOut := bytes.TrimSuffix(output, []byte("\n"))
	if !bytes.Equal(normalizedOut, normalizedWant) {
		tb.Fatalf("output mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", name, want, output)
	}
}

// NormalizeOutput trims trailing newline, strips \r (Windows CRLF),
// and right-trims trailing spaces from each rendered line.
func NormalizeOutput(output []byte) []byte {
	normalized := bytes.ReplaceAll(output, []byte("\r\n"), []byte("\n"))
	normalized = bytes.ReplaceAll(normalized, []byte("\r"), []byte("\n"))
	trimmedNewline := bytes.TrimSuffix(normalized, []byte("\n"))
	lines := strings.Split(string(trimmedNewline), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}

	return []byte(strings.Join(lines, "\n"))
}

// AssertMatchesGoldenNormalized compares normalized output against normalized golden text.
// Set env var TASKMGR_UI_UPDATE_GOLDEN=1 to write the current output as the new golden.
func AssertMatchesGoldenNormalized(tb testing.TB, output []byte, name string) {
	tb.Helper()

	got := NormalizeOutput(output)

	if os.Getenv("TASKMGR_UI_UPDATE_GOLDEN") == "1" {
		path := filepath.Join("testdata", name)
		if err := os.WriteFile(path, got, 0o600); err != nil {
			tb.Fatalf("write golden %s: %v", path, err)
		}
	}

	want := NormalizeOutput(ReadGolden(tb, name))
	if !bytes.Equal(got, want) {
		tb.Fatalf("output mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", name, string(want), string(got))
	}
}

// AssertModelViewMatchesGolden compares a model View() output to golden text.
func AssertModelViewMatchesGolden(tb testing.TB, model tea.Model, name string) {
	tb.Helper()

	AssertMatchesGolden(tb, []byte(model.View()), name)
}

// WaitForOutputContainsAll waits until output includes all snippets and returns matched render.
func WaitForOutputContainsAll(tb testing.TB, output io.Reader, snippets ...string) string {
	tb.Helper()
	return WaitForOutputContainsAllWithTimeout(tb, output, 0, snippets...)
}

// WaitForConditionWithTimeout polls cond every 10ms until it returns true or
// timeout elapses. It is intended for synchronising test goroutines on
// non-output state (e.g. checking a call counter on a fake). If timeout is
// zero, the function returns without waiting.
func WaitForConditionWithTimeout(tb testing.TB, timeout time.Duration, cond func() bool) {
	tb.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !cond() {
		tb.Fatalf("WaitForConditionWithTimeout: condition not met after %s", timeout)
	}
}

// WaitForOutputContainsAllWithTimeout is the same as WaitForOutputContainsAll
// but allows callers to extend the WaitFor budget. Pass timeout == 0 to use the
// teatest default. Use this for tests that exercise real taskmgr subprocesses under
// parallel `go test ./...` load, where the default 1s budget is insufficient.
func WaitForOutputContainsAllWithTimeout(tb testing.TB, output io.Reader, timeout time.Duration, snippets ...string) string {
	tb.Helper()

	var matched string
	opts := []teatest.WaitForOption{}
	if timeout > 0 {
		opts = append(opts, teatest.WithDuration(timeout))
	}
	teatest.WaitFor(tb, output, func(bts []byte) bool {
		view := string(bts)
		for _, snippet := range snippets {
			if !strings.Contains(view, snippet) {
				return false
			}
		}

		matched = view
		return true
	}, opts...)

	return matched
}
