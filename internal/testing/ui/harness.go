package ui

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
func AssertMatchesGolden(tb testing.TB, output []byte, name string) {
	tb.Helper()

	want := ReadGolden(tb, name)
	normalizedWant := bytes.TrimSuffix(want, []byte("\n"))
	normalizedOut := bytes.TrimSuffix(output, []byte("\n"))
	if !bytes.Equal(normalizedOut, normalizedWant) {
		tb.Fatalf("output mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", name, want, output)
	}
}

// NormalizeOutput trims trailing newline and right-trims each rendered line.
func NormalizeOutput(output []byte) []byte {
	trimmedNewline := bytes.TrimSuffix(output, []byte("\n"))
	lines := strings.Split(string(trimmedNewline), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}

	return []byte(strings.Join(lines, "\n"))
}

// AssertMatchesGoldenNormalized compares normalized output against normalized golden text.
func AssertMatchesGoldenNormalized(tb testing.TB, output []byte, name string) {
	tb.Helper()

	got := NormalizeOutput(output)
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

// AssertModelViewMatchesGoldenNormalized compares normalized model View() output to golden text.
func AssertModelViewMatchesGoldenNormalized(tb testing.TB, model tea.Model, name string) {
	tb.Helper()

	AssertMatchesGoldenNormalized(tb, []byte(model.View()), name)
}

// WaitForOutputContainsAll waits until output includes all snippets and returns matched render.
func WaitForOutputContainsAll(tb testing.TB, output io.Reader, snippets ...string) string {
	tb.Helper()

	var matched string
	teatest.WaitFor(tb, output, func(bts []byte) bool {
		view := string(bts)
		for _, snippet := range snippets {
			if !strings.Contains(view, snippet) {
				return false
			}
		}

		matched = view
		return true
	})

	return matched
}
