//go:build integration

package main

// Repo-wide documentation hygiene, not an architecture guardrail: this scan
// shells out to git, which is a real OS seam, so it is Tier 2 per
// docs/TESTING.md. Kept out of the unit suite because `git` being absent (a
// module-cache build, an extracted tarball, a minimal container) would
// otherwise fail every unit test in the package for a reason unrelated to the
// code under test.

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

func TestNoLeakedLocalTrackerIDs(t *testing.T) {
	t.Parallel()
	assertNoLeakedTrackerIDs(t)
}

// Tracker issue IDs must never appear in source comments or published docs —
// they reference an unpublished store that is not part of this repository, and
// a reader of the public repo cannot resolve them. Reference behavior by name
// instead.
//
// leakedTrackerIDPattern recognizes a tracker ID by one of a few high-precision
// shapes, chosen so the guard cannot misfire on ordinary code or text:
//
//   - the live `bwb-<id>` issue-key prefix (the current tracker namespace);
//   - a known legacy stem carrying the `task-manager-ui-` prefix or a `.<n>`
//     subtask suffix (e.g. vtvb.7, 0x36.4) — the prefix/suffix disambiguates it
//     from a hex literal or hash fragment; or
//   - a known legacy stem as a bare word, but ONLY the stems that are not valid
//     hex tokens. The hex-shaped stems (0x36, b38b, fbea) are excluded from
//     bare matching so a byte literal like `0x36` or a short-hash fragment never
//     trips the guard; every real leak of those appeared with a `.<n>`/prefix
//     shape, which is still caught.
//
// The legacy stem list is an explicit denylist of IDs known to have leaked.
// It deliberately omits the stems used by realistic issue-ID *test fixtures*
// (e.g. task-manager-ui-yze.4.2, -u5s, -9uk, -dim1, -sc1, -syf), so those
// legitimate fixtures are never flagged even though the scan reads raw text.
const (
	// Legacy stems safe to match as a bare standalone word (random, non-hex).
	trackerStemsBare = `iwvm|vtvb|ule7|czkq|znri|lgln|o7tk|uwmi|db0z|2ev4|okmo|6zvr|5q6t|j14c|24is|2rfx|h3ql`
	// All legacy stems, including the hex-shaped ones only matched with a
	// disambiguating prefix or `.<n>` suffix.
	trackerStemsAll = trackerStemsBare + `|0x36|b38b|fbea`
)

var leakedTrackerIDPattern = regexp.MustCompile(
	`\bbwb-[0-9a-z]{3,}` + // current tracker issue key (bwb-XXXXX placeholders are uppercase)
		`|task-manager-ui-(?:` + trackerStemsAll + `)` + // legacy prefixed form
		`|\b(?:` + trackerStemsAll + `)\.[0-9]+` + // legacy stem with a .<n> subtask suffix
		`|\b(?:` + trackerStemsBare + `)\b`, // legacy bare stem (non-hex only)
)

// assertNoLeakedTrackerIDs fails if any git-tracked .go or .md file contains a
// known local tracker ID. It scans the tracked set (via `git ls-files`) rather
// than walking the filesystem, so it sees exactly the committed source — the
// same view CI checks out — and never trips over gitignored build output (e.g.
// dist/ release notes) or local working dirs that may legitimately carry IDs.
// This guard file is skipped, since it holds the denylist literal.
func assertNoLeakedTrackerIDs(t *testing.T) {
	t.Helper()

	root := moduleRootDir(t)
	_, guardFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve guard test file path")
	}
	guardBase := filepath.Base(guardFile)

	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--", "*.go", "*.md").Output()
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
		for i, line := range strings.Split(string(content), "\n") {
			if hit := leakedTrackerIDPattern.FindString(line); hit != "" {
				violations = append(violations, fmt.Sprintf("%s:%d: leaked tracker ID %q", rel, i+1, hit))
			}
		}
	}

	if len(violations) == 0 {
		return
	}
	slices.Sort(violations)
	t.Fatalf("leaked tracker IDs found (reference behavior by name, not the tracker issue ID):\n%s", strings.Join(violations, "\n"))
}
