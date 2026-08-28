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

func TestDocsLinkAnchorsResolve(t *testing.T) {
	t.Parallel()
	assertDocLinkAnchorsResolve(t)
}

// A heading is a normal thing to rename, and the doc set cross-links heavily, so
// a link's `#fragment` rots on its own. TestDocsCiteLivePaths does not see it:
// that scan resolves paths, and a link whose target file still exists passes it
// while pointing at a heading nobody kept. Two such links shipped on main and
// were found only by reading the docs by hand.
var (
	// [text](target) — the target stops at the first whitespace or ')', which
	// keeps a link title (`[x](y "t")`) out of the captured path.
	markdownLinkPattern = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)

	// ATX headings only; the doc set uses no Setext (=== underline) headings.
	markdownHeadingPattern = regexp.MustCompile(`^\s{0,3}(#{1,6})\s+(.*?)\s*#*\s*$`)

	// [text](url) inside a heading contributes its text to the slug, not its URL.
	markdownLinkTextPattern = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)

	// Schemes whose fragments are somebody else's problem.
	externalLinkPattern = regexp.MustCompile(`(?i)^(https?:|mailto:|ftp:|//)`)
)

// assertDocLinkAnchorsResolve fails if a tracked Markdown file links to a
// `#fragment` that names no heading in the target document.
func assertDocLinkAnchorsResolve(t *testing.T) {
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

	headings := make(map[string]map[string]bool) // repo-relative path -> slug set
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
		violations = append(violations, linkAnchorViolations(root, rel, string(content), headings)...)
	}

	if len(violations) == 0 {
		return
	}
	slices.Sort(violations)
	t.Fatalf("dead documentation link anchors (the file resolves, the heading does not):\n%s",
		strings.Join(violations, "\n"))
}

func linkAnchorViolations(root, rel, content string, headings map[string]map[string]bool) []string {
	violations := make([]string, 0)
	lines := strings.Split(content, "\n")
	fenced := fencedLines(lines)

	for lineNo, line := range lines {
		if fenced[lineNo] {
			continue
		}
		for _, match := range markdownLinkPattern.FindAllStringSubmatch(line, -1) {
			target := match[1]
			if externalLinkPattern.MatchString(target) {
				continue
			}
			path, fragment, found := strings.Cut(target, "#")
			if !found || fragment == "" {
				continue // a plain path — TestDocsCiteLivePaths owns those
			}
			if strings.HasPrefix(fragment, "L") && isAllDigits(fragment[1:]) {
				continue // `#L42` line anchor — the citation scan reports it
			}

			// An empty path is a same-document anchor.
			targetRel := rel
			if path != "" {
				if !strings.HasSuffix(path, ".md") {
					continue // headings only exist in Markdown
				}
				targetRel = filepath.Clean(filepath.Join(filepath.Dir(rel), path))
			}

			slugs, loadErr := headingSlugs(root, targetRel, headings)
			if loadErr != nil {
				violations = append(violations, fmt.Sprintf(
					"%s:%d: links to %q, whose target file does not resolve", rel, lineNo+1, target))
				continue
			}
			if !slugs[strings.ToLower(fragment)] {
				violations = append(violations, fmt.Sprintf(
					"%s:%d: links to %q, but %s has no heading matching #%s",
					rel, lineNo+1, target, targetRel, fragment))
			}
		}
	}

	return violations
}

// headingSlugs returns the GitHub-style anchor slugs a document offers, reading
// and caching each target once. A cached nil marks a target that does not exist.
func headingSlugs(root, rel string, cache map[string]map[string]bool) (map[string]bool, error) {
	if slugs, ok := cache[rel]; ok {
		if slugs == nil {
			return nil, os.ErrNotExist
		}
		return slugs, nil
	}

	content, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		cache[rel] = nil
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	fenced := fencedLines(lines)
	slugs := make(map[string]bool)
	seen := make(map[string]int)
	for lineNo, line := range lines {
		if fenced[lineNo] {
			continue
		}
		m := markdownHeadingPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		base := githubHeadingSlug(m[2])
		if base == "" {
			continue
		}
		// GitHub disambiguates repeats by appending -1, -2, ...
		slug := base
		if n := seen[base]; n > 0 {
			slug = fmt.Sprintf("%s-%d", base, n)
		}
		seen[base]++
		slugs[slug] = true
	}

	cache[rel] = slugs
	return slugs, nil
}

// githubHeadingSlug lowercases, drops punctuation and turns spaces into hyphens,
// which is how GitHub derives a heading anchor. `## Launcher interpolation/context
// surface` becomes `launcher-interpolationcontext-surface` — the slash vanishes
// rather than becoming a separator, which is why this cannot be approximated by
// splitting on non-alphanumerics.
func githubHeadingSlug(heading string) string {
	s := markdownLinkTextPattern.ReplaceAllString(heading, "$1")
	s = strings.NewReplacer("`", "", "*", "", "~", "").Replace(s)
	s = strings.ToLower(strings.TrimSpace(s))

	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

// fencedLines marks the lines that sit inside a ``` or ~~~ fence, opening and
// closing markers included. A shell comment in a code block is not a heading,
// and a sample link in one is not a link the reader can follow.
func fencedLines(lines []string) map[int]bool {
	fenced := make(map[int]bool)
	open := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		marker := ""
		switch {
		case strings.HasPrefix(trimmed, "```"):
			marker = "```"
		case strings.HasPrefix(trimmed, "~~~"):
			marker = "~~~"
		}
		if marker != "" {
			fenced[i] = true
			if open == "" {
				open = marker
			} else if open == marker {
				open = ""
			}
			continue
		}
		if open != "" {
			fenced[i] = true
		}
	}
	return fenced
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
