package embeddedfixture

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Spec is the fixture seed specification consumed by the setup script.
type Spec struct {
	Prefix string           `json:"prefix"`
	Issues []IssueSpec      `json:"issues"`
	Deps   []DependencySpec `json:"dependencies"`
}

// IssueSpec describes one deterministic issue seed.
type IssueSpec struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Priority    int      `json:"priority"`
	Status      string   `json:"status"`
	Assignee    string   `json:"assignee"`
	Labels      []string `json:"labels"`
	Comments    []string `json:"comments"`
}

// DependencySpec states that BlockerID blocks BlockedID.
type DependencySpec struct {
	BlockerID string `json:"blocker_id"`
	BlockedID string `json:"blocked_id"`
}

// Paths returns package-relative paths to fixture assets.
func Paths(tb testing.TB) (scriptPath, seedPath string) {
	tb.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatal("failed to resolve embeddedfixture paths")
	}

	baseDir := filepath.Dir(file)
	return filepath.Join(baseDir, "setup.sh"), filepath.Join(baseDir, "seed.json")
}

// TempRepoPath returns a recommended temp directory path for fixture creation.
func TempRepoPath(tb testing.TB) string {
	tb.Helper()

	name := strings.ToLower(strings.ReplaceAll(tb.Name(), "/", "-"))
	return filepath.Join(tb.TempDir(), "embedded-beads-fixture-"+name)
}

// Seed creates and seeds an embedded-mode beads fixture repository.
//
// The fixture setup is delegated to setup.sh so integration and smoke tests can
// use the exact same reproducible data source.
func Seed(tb testing.TB, repoPath string) {
	tb.Helper()

	scriptPath, seedPath := Paths(tb)

	cmd := exec.Command("sh", scriptPath, repoPath, seedPath)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		tb.Fatalf("failed to seed embedded fixture repo %q: %v\n%s", repoPath, err, out)
	}

	if !strings.Contains(string(out), "fixture-ready:") {
		tb.Fatalf("unexpected fixture setup output: %s", out)
	}
}

// ReadSeedSpec loads the deterministic fixture seed specification.
func ReadSeedSpec(tb testing.TB) Spec {
	tb.Helper()

	_, seedPath := Paths(tb)
	bts, err := os.ReadFile(seedPath)
	if err != nil {
		tb.Fatalf("failed to read fixture seed %q: %v", seedPath, err)
	}

	var spec Spec
	if err := json.Unmarshal(bts, &spec); err != nil {
		tb.Fatalf("failed to decode fixture seed %q: %v", seedPath, err)
	}

	if spec.Prefix == "" {
		tb.Fatalf("invalid fixture seed %q: missing prefix", seedPath)
	}

	return spec
}
