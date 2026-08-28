//go:build integration

package main

// Repo-wide documentation hygiene, not an architecture guardrail: this scan
// shells out to git, which is a real OS seam, so it is Tier 2 per
// docs/TESTING.md — the same reason the tracker-ID scan next to it is tagged.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestDocsCiteLivePaths(t *testing.T) {
	t.Parallel()
	assertDocCitationsResolve(t)
}

// The doc set anchors its claims to code with paths like `internal/ui/scroll`.
// When code moves, those citations rot silently: the reader follows a dead path
// and the doc goes on reading as if it were true. This checks every cited path
// against the working tree.
//
// A `path:42` anchor rots faster than the path ever does — it survives no edit
// above it, and no check can see that it moved, because the file still exists.
// Cite the SYMBOL instead: it is stable across edits, and it is greppable, which
// a line number is not.
var (
	// Only these roots are checked. They are the trees that actually move;
	// a root dotfile (.mise.toml, .golangci.yml) does not.
	citationPattern = regexp.MustCompile(`(\./)?(internal|cmd|scripts|docs|\.github)/[A-Za-z0-9_.*/-]+`)

	// A trailing `.Symbol` names a Go identifier, not a path segment: the path
	// is what precedes it. An uppercase initial is the discriminator that keeps
	// `internal/version.Version` (package + symbol) apart from
	// `cmd/taskmgr-ui/main.go` (a real file).
	symbolSuffixPattern = regexp.MustCompile(`\.[A-Z][A-Za-z0-9_]*$`)

	// `path:42` and `path#L42`, measured from the character the citation ends on.
	lineAnchorPattern = regexp.MustCompile(`^(:\d+|#L\d+)`)
)

// assertDocCitationsResolve fails if any git-tracked Markdown file cites a
// repository path that no longer exists, or pins a citation to a line number.
// It scans the tracked set (via `git ls-files`) rather than walking the
// filesystem, so it sees exactly the committed docs — the same view CI checks
// out. This guard file is skipped, since it holds the pattern literals.
func assertDocCitationsResolve(t *testing.T) {
	t.Helper()

	root := moduleRootDir(t)
	_, guardFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve guard test file path")
	}
	guardBase := filepath.Base(guardFile)

	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--", "*.md").Output()
	if err != nil {
		t.Fatalf("listing tracked files via git failed: %v", err)
	}

	violations := make([]string, 0)
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel == "" || filepath.Base(rel) == guardBase {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(root, rel))
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue // tracked but absent in the working tree
			}
			t.Fatalf("reading %s failed: %v", rel, readErr)
		}
		violations = append(violations, citationViolations(t, root, rel, string(content))...)
	}

	if len(violations) == 0 {
		return
	}
	slices.Sort(violations)
	t.Fatalf("stale documentation citations (cite a path that exists, and a symbol rather than a line number):\n%s",
		strings.Join(violations, "\n"))
}

func citationViolations(t *testing.T, root, rel, content string) []string {
	t.Helper()

	violations := make([]string, 0)
	for lineNo, line := range strings.Split(content, "\n") {
		for _, span := range citationPattern.FindAllStringIndex(line, -1) {
			start, end := span[0], span[1]
			if start > 0 && continuesAPath(line[start-1]) {
				continue // the tail of a longer path, e.g. a full module import path
			}

			if lineAnchorPattern.MatchString(line[end:]) {
				violations = append(violations, fmt.Sprintf(
					"%s:%d: line-anchored citation %q — cite the symbol instead",
					rel, lineNo+1, line[start:end]+lineAnchorPattern.FindString(line[end:])))
				continue
			}

			cited := strings.TrimPrefix(strings.TrimRight(line[start:end], "./-"), "./")
			if cited == "" || strings.Contains(cited, "..") {
				continue
			}
			if path := symbolSuffixPattern.ReplaceAllString(cited, ""); path != cited {
				cited = path
			}

			if !pathExists(root, cited) {
				violations = append(violations, fmt.Sprintf("%s:%d: cites %q, which does not exist", rel, lineNo+1, cited))
			}
		}
	}

	return violations
}

// continuesAPath reports whether c would make the character before a match part
// of the same path token — the check that keeps the `internal/version` tail of
// `github.com/hk9890/task-manager-ui/internal/version` from being read as a
// citation in its own right.
func continuesAPath(c byte) bool {
	return c == '/' || c == '.' || c == '-' || c == '_' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// pathExists resolves one citation. A citation carrying a `*` is a shape rather
// than a path (`internal/mode/*`, `internal/ui/*/*_test.go`), so it passes when
// the shape still matches something.
func pathExists(root, cited string) bool {
	full := filepath.Join(root, cited)
	if strings.Contains(cited, "*") {
		matches, err := filepath.Glob(full)
		return err == nil && len(matches) > 0
	}
	_, err := os.Stat(full)

	return err == nil
}
