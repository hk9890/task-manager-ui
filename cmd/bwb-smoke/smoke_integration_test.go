//go:build integration

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestSmokeIntegration builds the bwb-smoke binary and runs it against the
// embedded fixture, asserting exit code 0 (all checks PASS).
func TestSmokeIntegration(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not found on PATH; skipping smoke integration test")
	}

	// Locate the fixture setup script and seed file via source path.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := findRepoRoot(t, filepath.Dir(thisFile))

	// Build the binary into a temp dir.
	binPath := filepath.Join(t.TempDir(), "bwb-smoke")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/bwb-smoke")
	buildCmd.Dir = repoRoot
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/bwb-smoke failed: %v\n%s", err, out)
	}

	// Seed the embedded fixture into a fresh temp directory.
	fixtureDir := t.TempDir()
	setupScript := filepath.Join(repoRoot, "internal/testing/e2e/embeddedfixture/setup.sh")
	seedFile := filepath.Join(repoRoot, "internal/testing/e2e/embeddedfixture/seed.json")
	setupCmd := exec.Command("sh", setupScript, fixtureDir, seedFile)
	setupCmd.Dir = repoRoot
	if out, err := setupCmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture setup failed: %v\n%s", err, out)
	}

	// Run bwb-smoke --dir <fixture> --readonly --format json.
	smokeCmd := exec.Command(binPath,
		"--dir", fixtureDir,
		"--readonly=false", // fixture is writable; using false avoids --readonly prepend
		"--format", "json",
	)
	smokeCmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
	out, err := smokeCmd.Output()

	// Decode exit code.
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("bwb-smoke error: %v", err)
		}
	}

	// Parse output.
	var report jsonReport
	if jsonErr := json.Unmarshal(out, &report); jsonErr != nil {
		t.Fatalf("bwb-smoke output is not valid JSON: %v\nraw: %s", jsonErr, out)
	}

	t.Logf("bwb-smoke exit code: %d", exitCode)
	t.Logf("bwb-smoke result: %s", report.Result)
	for _, c := range report.Checks {
		t.Logf("  %-10s  %-6s  %s", c.Name, c.Status, c.Detail)
	}

	if exitCode != 0 {
		t.Errorf("bwb-smoke exited with code %d (result=%s)", exitCode, report.Result)
		for _, c := range report.Checks {
			if c.Status != "PASS" {
				t.Errorf("  FAIL check %q: %s", c.Name, c.Detail)
			}
		}
	}

	// Verify the JSON shape: must have dir, checks array, result string.
	if report.Dir == "" {
		t.Error("JSON missing dir field")
	}
	if len(report.Checks) == 0 {
		t.Error("JSON checks array is empty")
	}
	if report.Result != "PASS" && report.Result != "FAIL" {
		t.Errorf("JSON result must be PASS or FAIL, got %q", report.Result)
	}
}

// TestSmokeIntegrationJSONPipeable runs the smoke binary and verifies the JSON
// output is parseable by the standard json package (simulates piping to jq).
func TestSmokeIntegrationJSONPipeable(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not found on PATH; skipping")
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := findRepoRoot(t, filepath.Dir(thisFile))

	binPath := filepath.Join(t.TempDir(), "bwb-smoke")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/bwb-smoke")
	buildCmd.Dir = repoRoot
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// Run against this repo (readonly).
	thisRepoDir := repoRoot
	smokeCmd := exec.Command(binPath,
		"--dir", thisRepoDir,
		"--readonly",
		"--format", "json",
	)
	smokeCmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
	out, err := smokeCmd.Output()
	// Non-zero exit is acceptable (parity checks may FAIL on unpatched db).
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("bwb-smoke fatal error: %v", err)
		}
	}

	// Output must be valid JSON regardless of exit code.
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		t.Fatal("bwb-smoke produced no output")
	}
	var v any
	if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
		t.Errorf("output is not valid JSON: %v\nraw: %s", err, out)
	}
}

func findRepoRoot(t *testing.T, dir string) string {
	t.Helper()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("findRepoRoot: go.mod not found above %q", dir)
		}
		dir = parent
	}
}
