package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// stubTB records what a golden helper reports instead of aborting the run, so a
// deliberate mismatch can be asserted rather than failing the test that caused
// it. testing.TB carries an unexported method, so embedding the interface is
// the only way to satisfy it; the embedded value stays nil on purpose, so a
// helper that starts calling some other method panics here rather than passing
// silently.
type stubTB struct {
	testing.TB
	failures []string
}

func (s *stubTB) Helper() {}

func (s *stubTB) Fatalf(format string, args ...any) {
	s.failures = append(s.failures, fmt.Sprintf(format, args...))
}

func (s *stubTB) Errorf(format string, args ...any) {
	s.failures = append(s.failures, fmt.Sprintf(format, args...))
}

// goldenAsserters are the three comparators that share updateGolden. Each is
// exercised against the same plain-ASCII bytes, for which all three write the
// identical byte stream — the equality the updateGolden doc comment promises.
var goldenAsserters = map[string]func(testing.TB, []byte, string){
	"AssertMatchesGolden":           AssertMatchesGolden,
	"AssertMatchesGoldenNormalized": AssertMatchesGoldenNormalized,
	"AssertMatchesGoldenStripANSI":  AssertMatchesGoldenStripANSI,
}

// seedGoldenDir points the relative testdata path updateGolden writes to at a
// scratch directory, so a regeneration cannot reach a committed golden. t.Chdir
// forbids a parallel test, which is why nothing here calls t.Parallel: the
// sequential tests in a package all finish before the parallel ones resume.
func seedGoldenDir(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Mkdir("testdata", 0o750); err != nil {
		t.Fatalf("create testdata dir: %v", err)
	}
	path := filepath.Join("testdata", "gate.golden")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("seed golden: %v", err)
	}

	return path
}

func TestGoldenAssertionsReportMismatchAndLeaveTheGoldenUnchanged(t *testing.T) {
	for name, assert := range goldenAsserters {
		t.Run(name, func(t *testing.T) {
			// Forced empty rather than merely unset: a regeneration run
			// (TASKMGR_UI_UPDATE_GOLDEN=1 go test ./internal/...) must not
			// change what this test asserts.
			t.Setenv("TASKMGR_UI_UPDATE_GOLDEN", "")
			path := seedGoldenDir(t, "expected")

			stub := &stubTB{}
			assert(stub, []byte("actual"), "gate.golden")

			if len(stub.failures) == 0 {
				t.Fatal("mismatch was not reported: the golden comparison passed on differing bytes")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden back: %v", err)
			}
			if string(got) != "expected" {
				t.Fatalf("golden was rewritten with the update gate closed: got %q, want %q", got, "expected")
			}
		})
	}
}

func TestGoldenAssertionsRegenerateOnlyWhenTheUpdateGateIsOpen(t *testing.T) {
	for name, assert := range goldenAsserters {
		t.Run(name, func(t *testing.T) {
			t.Setenv("TASKMGR_UI_UPDATE_GOLDEN", "1")
			path := seedGoldenDir(t, "stale")

			stub := &stubTB{}
			assert(stub, []byte("fresh"), "gate.golden")

			if len(stub.failures) != 0 {
				t.Fatalf("regeneration reported a failure: %v", stub.failures)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden back: %v", err)
			}
			if string(got) != "fresh" {
				t.Fatalf("golden was not regenerated with the update gate open: got %q, want %q", got, "fresh")
			}
		})
	}
}
