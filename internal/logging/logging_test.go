package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestResolveLogPathCreatesBWBStateDirectory(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()

	logPath, err := resolveLogPath(stateDir)
	if err != nil {
		t.Fatalf("resolveLogPath returned error: %v", err)
	}

	expectedDir := filepath.Join(stateDir, stateDirName)
	if filepath.Dir(logPath) != expectedDir {
		t.Fatalf("expected log parent %q, got %q", expectedDir, filepath.Dir(logPath))
	}
	if filepath.Base(logPath) != defaultLogFileName {
		t.Fatalf("expected log file name %q, got %q", defaultLogFileName, filepath.Base(logPath))
	}

	info, err := os.Stat(expectedDir)
	if err != nil {
		t.Fatalf("expected state directory to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %q to be a directory", expectedDir)
	}
}

func TestManagerJSONRecordShapeAndComponentScope(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	var stderr bytes.Buffer

	m := New(Options{
		StateDir:     stateDir,
		Stderr:       &stderr,
		SessionID:    "deadbeef",
		ProjectRoot:  "/tmp/project-a",
		BuildVersion: "dev",
	})
	t.Cleanup(func() {
		_ = m.Close()
	})

	m.Component("gateway").Warn("gateway warning", "argv", []string{"bd", "ready", "--json"})

	line := firstLineFromFile(t, m.LogPath())
	record := decodeJSONLine(t, line)

	for _, key := range []string{"timestamp", "level", "message", "session_id", "project_root", "build_version"} {
		if _, ok := record[key]; !ok {
			t.Fatalf("expected JSON key %q in record: %#v", key, record)
		}
	}

	if got := record["level"]; got != "WARN" {
		t.Fatalf("expected level WARN, got %#v", got)
	}
	if got := record["message"]; got != "gateway warning" {
		t.Fatalf("expected message %q, got %#v", "gateway warning", got)
	}
	if got := record["session_id"]; got != "deadbeef" {
		t.Fatalf("expected session_id deadbeef, got %#v", got)
	}
	if got := record["project_root"]; got != "/tmp/project-a" {
		t.Fatalf("expected project_root /tmp/project-a, got %#v", got)
	}
	if got := record["build_version"]; got != "dev" {
		t.Fatalf("expected build_version dev, got %#v", got)
	}
	if got := record["component"]; got != "gateway" {
		t.Fatalf("expected component gateway, got %#v", got)
	}

	if got := stderr.String(); !strings.Contains(got, "warn: gateway warning") {
		t.Fatalf("expected warning mirrored to stderr, got %q", got)
	}
	if got := stderr.String(); !strings.Contains(got, "project_root=/tmp/project-a") || !strings.Contains(got, "build_version=dev") {
		t.Fatalf("expected provenance mirrored to stderr, got %q", got)
	}
}

func TestManagerDebugLogsMirrorToStderrWithPrefix(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	var stderr bytes.Buffer

	m := New(Options{
		StateDir:     stateDir,
		Stderr:       &stderr,
		Debug:        true,
		SessionID:    "cafebabe",
		ProjectRoot:  "/tmp/project-b",
		BuildVersion: "1.2.3",
	})
	t.Cleanup(func() {
		_ = m.Close()
	})

	m.Component("gateway").Debug("bd argv trace", "argv", []string{"bd", "show", "ISSUE-1"})

	gotStderr := stderr.String()
	if !strings.Contains(gotStderr, "[bwb-debug] session_id=cafebabe") {
		t.Fatalf("expected debug session line, got %q", gotStderr)
	}
	if !strings.Contains(gotStderr, "[bwb-debug] bd argv trace") {
		t.Fatalf("expected debug message with prefix, got %q", gotStderr)
	}
	if !strings.Contains(gotStderr, "project_root=/tmp/project-b") || !strings.Contains(gotStderr, "build_version=1.2.3") {
		t.Fatalf("expected debug provenance fields, got %q", gotStderr)
	}

	line := firstLineFromFile(t, m.LogPath())
	record := decodeJSONLine(t, line)
	if got := record["level"]; got != "DEBUG" {
		t.Fatalf("expected DEBUG level in persistent log, got %#v", got)
	}
	if got := record["message"]; got != "bd argv trace" {
		t.Fatalf("expected debug message in persistent log, got %#v", got)
	}
	if got := record["project_root"]; got != "/tmp/project-b" {
		t.Fatalf("expected project_root /tmp/project-b, got %#v", got)
	}
	if got := record["build_version"]; got != "1.2.3" {
		t.Fatalf("expected build_version 1.2.3, got %#v", got)
	}
}

func TestManagerInfoLogsMirrorToStderrWithDebugPrefixWhenDebugEnabled(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	var stderr bytes.Buffer

	m := New(Options{
		StateDir:     stateDir,
		Stderr:       &stderr,
		Debug:        true,
		SessionID:    "facefeed",
		ProjectRoot:  "/tmp/project-c",
		BuildVersion: "dev",
	})
	t.Cleanup(func() {
		_ = m.Close()
	})

	m.Component("startup").Info("resolved config path", "path", "/tmp/cfg.yaml")

	gotStderr := stderr.String()
	if !strings.Contains(gotStderr, "[bwb-debug] resolved config path") || !strings.Contains(gotStderr, "path=/tmp/cfg.yaml") {
		t.Fatalf("expected info message with debug prefix, got %q", gotStderr)
	}

	line := firstLineFromFile(t, m.LogPath())
	record := decodeJSONLine(t, line)
	if got := record["level"]; got != "INFO" {
		t.Fatalf("expected INFO level in persistent log, got %#v", got)
	}
	if got := record["message"]; got != "resolved config path" {
		t.Fatalf("expected info message in persistent log, got %#v", got)
	}
	if got := record["project_root"]; got != "/tmp/project-c" {
		t.Fatalf("expected project_root /tmp/project-c, got %#v", got)
	}
	if got := record["build_version"]; got != "dev" {
		t.Fatalf("expected build_version dev, got %#v", got)
	}
}

func TestForcedRotationProducesRotatedOutput(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	logPath, sink, err := buildPersistentSink(Options{StateDir: stateDir})
	if err != nil {
		t.Fatalf("buildPersistentSink returned error: %v", err)
	}
	// Close the sink and wait for lumberjack's background compression goroutine
	// to finish before t.TempDir cleanup removes the directory. lumberjack.Close
	// only closes the active log file; it does not drain the mill goroutine that
	// compresses rotated backups. Without this wait, the goroutine may still hold
	// files open in the temp dir when Go's test harness calls TempDir RemoveAll,
	// causing an intermittent "directory not empty" error (~1 in 50 runs).
	t.Cleanup(func() {
		_ = sink.Close()
		waitForLumberjackMill(t, filepath.Dir(logPath), 5*time.Second)
	})

	if _, err := sink.Write([]byte("before-rotate\n")); err != nil {
		t.Fatalf("write before rotate failed: %v", err)
	}
	if err := sink.Rotate(); err != nil {
		t.Fatalf("forced rotate failed: %v", err)
	}
	if _, err := sink.Write([]byte("after-rotate\n")); err != nil {
		t.Fatalf("write after rotate failed: %v", err)
	}

	entries, err := os.ReadDir(filepath.Dir(logPath))
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	rotatedFound := false
	for _, entry := range entries {
		name := entry.Name()
		if name == defaultLogFileName {
			continue
		}
		if strings.Contains(name, ".log") {
			rotatedFound = true
			break
		}
	}

	if !rotatedFound {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("expected rotated output file to be produced, entries=%v", names)
	}
}

// waitForLumberjackMill polls logDir until no uncompressed backup log files
// remain (i.e. lumberjack's background mill goroutine has finished compressing
// all rotated backups), or until timeout elapses. It is a best-effort guard
// against the race between lumberjack's compression goroutine and t.TempDir
// cleanup — lumberjack.Logger.Close does not drain the mill goroutine.
func waitForLumberjackMill(t *testing.T, logDir string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(logDir)
		if err != nil {
			// Directory already gone — nothing to wait for.
			return
		}
		pendingCompression := false
		for _, entry := range entries {
			name := entry.Name()
			if name == defaultLogFileName {
				continue
			}
			// A backup file that has ".log" but not ".gz" suffix is still being
			// compressed (or waiting to be compressed) by the mill goroutine.
			if strings.HasSuffix(name, ".log") {
				pendingCompression = true
				break
			}
		}
		if !pendingCompression {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Timeout elapsed — log a warning but do not fail; the test assertions
	// already passed. A leftover file may still cause TempDir cleanup noise
	// but that is non-fatal for the test result.
	t.Logf("waitForLumberjackMill: timeout waiting for compression in %s", logDir)
}

func TestFallbackWhenStateDirUnavailableWarnsOnceAndUsesStderrOnly(t *testing.T) {
	t.Parallel()

	notDirectory := filepath.Join(t.TempDir(), "state-file")
	if err := os.WriteFile(notDirectory, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	var stderr bytes.Buffer
	m := New(Options{StateDir: notDirectory, Stderr: &stderr})

	if m.LogPath() != "" {
		t.Fatalf("expected empty log path in fallback mode, got %q", m.LogPath())
	}

	m.Logger().Warn("fallback warning")

	got := stderr.String()
	if count := strings.Count(got, "persistent log unavailable"); count != 1 {
		t.Fatalf("expected one fallback warning, got %d in %q", count, got)
	}
	if !strings.Contains(got, "warn: fallback warning") {
		t.Fatalf("expected warnings to continue on stderr, got %q", got)
	}
}

// TestManagerCloseDoesNotPanicWhenPersistentSinkUnavailable is a regression
// test for the typed-nil interface bug: when buildPersistentSink fails, a
// previous version of New assigned the (*lumberjack.Logger)(nil) return value
// directly to the io.Closer interface field. The interface was non-nil (it had
// a type but a nil pointer value), so the nil guard in Close was bypassed and
// lumberjack.Logger.Close panicked with a nil dereference.
//
// This test must FAIL on the pre-fix code (closer == typed nil interface) and
// PASS after the fix (closer == nil interface when sink construction fails).
func TestManagerCloseDoesNotPanicWhenPersistentSinkUnavailable(t *testing.T) {
	t.Parallel()

	// Use a plain file path as the state dir so MkdirAll fails — simulating an
	// environment where the log state directory cannot be created (e.g. HOME=/root
	// with no write permission).
	notDirectory := filepath.Join(t.TempDir(), "state-file")
	if err := os.WriteFile(notDirectory, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	var stderr bytes.Buffer
	m := New(Options{StateDir: notDirectory, Stderr: &stderr})

	// Close must not panic. On the buggy code this triggers a nil-dereference
	// inside lumberjack.Logger.Close via the non-nil typed-nil interface value.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Manager.Close panicked when persistent sink unavailable: %v", r)
		}
	}()

	if err := m.Close(); err != nil {
		t.Fatalf("expected nil error from Close in fallback mode, got: %v", err)
	}
}

func TestGenerateSessionIDReturnsHexOnSuccess(t *testing.T) {
	t.Parallel()

	id := generateSessionID()
	if len(id) == 0 {
		t.Fatal("expected non-empty session ID")
	}
	// Must be valid hex.
	if _, err := strconv.ParseUint(id, 16, 64); err != nil {
		// More than 64 bits: just verify every character is a hex digit.
		for _, c := range id {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				t.Fatalf("session ID %q contains non-hex character %q", id, c)
			}
		}
	}
}

func TestGenerateSessionIDFallsBackToTimestampOnRandError(t *testing.T) {
	// Not parallel: modifies package-level randReader.
	orig := randReader
	randReader = func(b []byte) (int, error) { return 0, errors.New("entropy exhausted") }
	t.Cleanup(func() { randReader = orig })

	id1 := generateSessionID()
	id2 := generateSessionID()

	if id1 == "00000000" {
		t.Fatalf("expected non-literal fallback, got %q", id1)
	}
	// Both IDs must be non-empty hex strings.
	for _, id := range []string{id1, id2} {
		if len(id) == 0 {
			t.Fatalf("expected non-empty fallback session ID")
		}
		for _, c := range id {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				t.Fatalf("fallback session ID %q contains non-hex character %q", id, c)
			}
		}
	}
}

// TestSetStderrSuppressedBlocksStderrButNotPersistentLog verifies that
// Manager.SetStderrSuppressed(true) prevents warn+ records from reaching
// stderr while the persistent file sink continues to receive all records.
// This is the regression guard for the alt-screen corruption bug (o7tk root
// cause 2): if suppression is removed, the first and third assertions will
// pass but the second will fail because the warn line appears on stderr again.
func TestSetStderrSuppressedBlocksStderrButNotPersistentLog(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	var stderr bytes.Buffer

	m := New(Options{
		StateDir:     stateDir,
		Stderr:       &stderr,
		SessionID:    "supptest1",
		ProjectRoot:  "/tmp/project-suppress",
		BuildVersion: "dev",
	})
	t.Cleanup(func() { _ = m.Close() })

	// Phase 1: suppression off — warn must reach stderr AND persistent file.
	m.Logger().Warn("before-suppress")
	if !strings.Contains(stderr.String(), "warn: before-suppress") {
		t.Fatalf("phase 1: expected warn to reach stderr before suppression, got %q", stderr.String())
	}

	// Phase 2: suppression on — warn must NOT reach stderr; file still receives it.
	m.SetStderrSuppressed(true)
	stderr.Reset()
	m.Logger().Warn("during-suppress")
	if strings.Contains(stderr.String(), "during-suppress") {
		t.Fatalf("phase 2 (regression): warn reached stderr while suppressed — alt-screen corruption would occur; got %q", stderr.String())
	}

	// Phase 3: suppression off again — warn must reach stderr again.
	m.SetStderrSuppressed(false)
	m.Logger().Warn("after-suppress")
	if !strings.Contains(stderr.String(), "warn: after-suppress") {
		t.Fatalf("phase 3: expected warn to reach stderr after unsuppression, got %q", stderr.String())
	}

	// All three messages must be in the persistent log.
	content, err := os.ReadFile(m.LogPath())
	if err != nil {
		t.Fatalf("ReadFile log: %v", err)
	}
	for _, msg := range []string{"before-suppress", "during-suppress", "after-suppress"} {
		if !strings.Contains(string(content), msg) {
			t.Fatalf("persistent log missing message %q; log content: %s", msg, string(content))
		}
	}
}

// TestSetStderrSuppressedDebugModeAlsoSuppressed verifies that --debug mode
// (which enables info/debug writes to stderr) is also suppressed during
// interactive mode. No stderr writes may corrupt the alt-screen even in debug.
func TestSetStderrSuppressedDebugModeAlsoSuppressed(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	var stderr bytes.Buffer

	m := New(Options{
		StateDir:     stateDir,
		Stderr:       &stderr,
		Debug:        true,
		SessionID:    "supptest2",
		ProjectRoot:  "/tmp/project-debug-suppress",
		BuildVersion: "dev",
	})
	t.Cleanup(func() { _ = m.Close() })

	// Confirm debug info reaches stderr before suppression.
	stderr.Reset() // clear the session_id line emitted by New
	m.Logger().Info("pre-suppress-info")
	if !strings.Contains(stderr.String(), "pre-suppress-info") {
		t.Fatalf("expected info to reach stderr in debug mode before suppression, got %q", stderr.String())
	}

	// Suppress and verify nothing reaches stderr.
	m.SetStderrSuppressed(true)
	stderr.Reset()
	m.Logger().Info("suppressed-info")
	m.Logger().Warn("suppressed-warn")
	m.Logger().Debug("suppressed-debug")
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr while suppressed in debug mode, got %q", stderr.String())
	}
}

// TestSetStderrSuppressedNilManagerIsNoop verifies that calling
// SetStderrSuppressed on a nil Manager does not panic.
func TestSetStderrSuppressedNilManagerIsNoop(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetStderrSuppressed on nil Manager panicked: %v", r)
		}
	}()

	var m *Manager
	m.SetStderrSuppressed(true)
	m.SetStderrSuppressed(false)
}

func firstLineFromFile(t *testing.T, filePath string) string {
	t.Helper()

	if strings.TrimSpace(filePath) == "" {
		t.Fatal("expected non-empty log path")
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	text := strings.TrimSpace(string(content))
	if text == "" {
		t.Fatalf("expected log file content at %q", filePath)
	}
	lines := strings.Split(text, "\n")
	return lines[0]
}

func decodeJSONLine(t *testing.T, line string) map[string]any {
	t.Helper()

	var out map[string]any
	if err := json.Unmarshal([]byte(line), &out); err != nil {
		t.Fatalf("json.Unmarshal failed for %q: %v", line, err)
	}
	return out
}
