package logging

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		StateDir:  stateDir,
		Stderr:    &stderr,
		SessionID: "deadbeef",
	})
	t.Cleanup(func() {
		_ = m.Close()
	})

	m.Component("gateway").Warn("gateway warning", "argv", []string{"bd", "ready", "--json"})

	line := firstLineFromFile(t, m.LogPath())
	record := decodeJSONLine(t, line)

	for _, key := range []string{"timestamp", "level", "message", "session_id"} {
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
	if got := record["component"]; got != "gateway" {
		t.Fatalf("expected component gateway, got %#v", got)
	}

	if got := stderr.String(); !strings.Contains(got, "warn: gateway warning") {
		t.Fatalf("expected warning mirrored to stderr, got %q", got)
	}
}

func TestManagerDebugLogsMirrorToStderrWithPrefix(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	var stderr bytes.Buffer

	m := New(Options{
		StateDir:  stateDir,
		Stderr:    &stderr,
		Debug:     true,
		SessionID: "cafebabe",
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

	line := firstLineFromFile(t, m.LogPath())
	record := decodeJSONLine(t, line)
	if got := record["level"]; got != "DEBUG" {
		t.Fatalf("expected DEBUG level in persistent log, got %#v", got)
	}
	if got := record["message"]; got != "bd argv trace" {
		t.Fatalf("expected debug message in persistent log, got %#v", got)
	}
}

func TestManagerInfoLogsMirrorToStderrWithDebugPrefixWhenDebugEnabled(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	var stderr bytes.Buffer

	m := New(Options{
		StateDir:  stateDir,
		Stderr:    &stderr,
		Debug:     true,
		SessionID: "facefeed",
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
}

func TestForcedRotationProducesRotatedOutput(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	logPath, sink, err := buildPersistentSink(Options{StateDir: stateDir})
	if err != nil {
		t.Fatalf("buildPersistentSink returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = sink.Close()
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
