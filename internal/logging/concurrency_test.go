package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestStderrHandlerDerivedHandlersShareMutex verifies that handlers produced
// by WithAttrs and WithGroup share the same *sync.Mutex pointer as the root.
func TestStderrHandlerDerivedHandlersShareMutex(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	root := newStderrHandler(&buf, true).(*stderrHandler)

	derived1 := root.WithAttrs([]slog.Attr{slog.String("k", "v")}).(*stderrHandler)
	derived2 := root.WithGroup("g").(*stderrHandler)
	derived3 := derived1.WithGroup("g2").(*stderrHandler)

	if root.mu != derived1.mu {
		t.Error("WithAttrs should copy the mutex pointer, not allocate a fresh one")
	}
	if root.mu != derived2.mu {
		t.Error("WithGroup should copy the mutex pointer, not allocate a fresh one")
	}
	if root.mu != derived3.mu {
		t.Error("chained WithGroup should still share the original mutex pointer")
	}
}

// TestStderrHandlerConcurrentWritesNoInterleavedLines exercises 10 goroutines
// each emitting 100 warn records through derived handlers that share the same
// io.Writer. Every line must be a complete record starting with "warn:".
func TestStderrHandlerConcurrentWritesNoInterleavedLines(t *testing.T) {
	t.Parallel()

	var buf safeBuffer
	root := newStderrHandler(&buf, false)

	const goroutines = 10
	const recsPerGoroutine = 100

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		g := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each goroutine uses its own derived handler, simulating Component loggers.
			h := root.WithAttrs([]slog.Attr{slog.Int("goroutine", g)})
			for i := 0; i < recsPerGoroutine; i++ {
				rec := slog.NewRecord(time.Now(), slog.LevelWarn, fmt.Sprintf("msg g=%d i=%d", g, i), 0)
				_ = h.Handle(t.Context(), rec)
			}
		}()
	}
	wg.Wait()

	output := buf.String()
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	total := goroutines * recsPerGoroutine
	if len(lines) != total {
		t.Fatalf("expected %d lines, got %d\nfirst 5 lines:\n%s",
			total, len(lines), strings.Join(lines[:minInt(5, len(lines))], "\n"))
	}

	for i, line := range lines {
		if !strings.HasPrefix(line, "warn:") {
			t.Errorf("line %d does not start with 'warn:': %q", i, line)
		}
	}
}

// TestConcurrentJSONFileHandlerNoInterleavedLines exercises 10 goroutines
// writing through the full Manager tee path. Every line in the log file must
// parse as valid JSON.
func TestConcurrentJSONFileHandlerNoInterleavedLines(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	var stderr bytes.Buffer

	m := New(Options{
		StateDir:     stateDir,
		Stderr:       &stderr,
		SessionID:    "conctest",
		ProjectRoot:  "/tmp/proj",
		BuildVersion: "dev",
		Debug:        false,
	})
	t.Cleanup(func() { _ = m.Close() })

	const goroutines = 10
	const recsPerGoroutine = 100

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		g := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger := m.Component(fmt.Sprintf("comp-%d", g))
			for i := 0; i < recsPerGoroutine; i++ {
				logger.Warn("concurrent record", "g", g, "i", i)
			}
		}()
	}
	wg.Wait()

	content, err := os.ReadFile(m.LogPath())
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", m.LogPath(), err)
	}

	lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")

	if len(lines) != goroutines*recsPerGoroutine {
		t.Fatalf("expected %d JSON lines, got %d", goroutines*recsPerGoroutine, len(lines))
	}
	for i, line := range lines {
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Errorf("line %d is not valid JSON (err: %v): %q", i, err, line)
		}
	}
}

// TestFailsafeSinkWriteFailureSurfacesOneWarning provides a fake writer that
// always returns ENOSPC and verifies:
//  1. Exactly one warning line is emitted to the fallback writer.
//  2. Subsequent writes still reach the fallback (logging continues).
func TestFailsafeSinkWriteFailureSurfacesOneWarning(t *testing.T) {
	t.Parallel()

	primary := &errWriter{err: syscall.ENOSPC}
	var fallback bytes.Buffer

	sink := newFailsafeSink(primary, &fallback)

	_, _ = sink.Write([]byte("record-1\n"))
	_, _ = sink.Write([]byte("record-2\n"))
	_, _ = sink.Write([]byte("record-3\n"))

	got := fallback.String()

	warnCount := strings.Count(got, "persistent sink write failed")
	if warnCount != 1 {
		t.Errorf("expected exactly 1 warning line, got %d in output:\n%s", warnCount, got)
	}

	for _, rec := range []string{"record-1", "record-2", "record-3"} {
		if !strings.Contains(got, rec) {
			t.Errorf("expected %q in fallback output, got:\n%s", rec, got)
		}
	}

	if primary.calls > 1 {
		t.Errorf("expected primary writer called at most once, got %d calls", primary.calls)
	}
}

// TestFailsafeSinkConcurrentWriteFailure confirms that exactly one warning is
// emitted even when multiple goroutines race to trigger the first failure.
func TestFailsafeSinkConcurrentWriteFailure(t *testing.T) {
	t.Parallel()

	primary := &errWriter{err: syscall.ENOSPC}
	var fallback safeBuffer

	sink := newFailsafeSink(primary, &fallback)

	const goroutines = 20
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = sink.Write([]byte("concurrent-record\n"))
		}()
	}
	wg.Wait()

	got := fallback.String()
	warnCount := strings.Count(got, "persistent sink write failed")
	if warnCount != 1 {
		t.Errorf("expected exactly 1 warning under concurrent failure, got %d in output:\n%s", warnCount, got)
	}
}

// errWriter always returns the configured error on Write.
type errWriter struct {
	mu    sync.Mutex
	err   error
	calls int
}

func (w *errWriter) Write(_ []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls++
	return 0, w.err
}

// safeBuffer is a bytes.Buffer protected by a mutex for concurrent use in tests.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
