//go:build integration

package embeddedfixture

// scale_fixture_smoke_integration_test.go — repository-bound smoke invariants for
// the scale fixture (beads-workbench-faif.1).
//
// These tests fork real bd subprocesses and require the scale fixture to be
// seeded into a live beads database.  They live behind the integration build tag
// and run only with `mise run test:integration`.
//
// IMPORTANT: seeding scale-seed.json (~590 issues) via setup.sh takes several
// minutes due to the number of bd create/close subprocess calls.  These tests
// are therefore opt-in: set BWB_SCALE_FIXTURE_SMOKE=1 to enable them.
//
// Without the env var, all tests in this file are skipped immediately.
//
// NOTE: These tests use SharedScaleFixtureRepoPath (faif.4 shared cache) so
// the scale fixture is seeded once per process rather than once per test.

import (
	"context"
	"os"
	"testing"

	bd "github.com/hk9890/beads-workbench/internal/bd"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/repository"
	repobeads "github.com/hk9890/beads-workbench/internal/repository/beads"
)

// checkScaleGateEnabled skips the test unless BWB_SCALE_FIXTURE_SMOKE=1.
func checkScaleGateEnabled(tb testing.TB) {
	tb.Helper()
	if os.Getenv("BWB_SCALE_FIXTURE_SMOKE") != "1" {
		tb.Skip("scale fixture repository smoke: set BWB_SCALE_FIXTURE_SMOKE=1 to enable (seeding ~590 issues takes several minutes)")
	}
}

// newScaleRepository builds a lean Repository pointing at repoPath.
func newScaleRepository(repoPath string) repository.Repository {
	runner := bd.NewCommandRunner(bd.RunnerConfig{
		WorkDir: repoPath,
	})
	return repobeads.New(runner)
}

// TestScaleFixtureRepository_SearchKeywordReturnsGe20Results guards the search
// corpus: searching for a shared keyword must return >=20 results in a
// deterministic order (no panic, no empty result for a well-populated corpus).
func TestScaleFixtureRepository_SearchKeywordReturnsGe20Results(t *testing.T) {
	// regression class: search corpus / deterministic search order
	//
	// Uses SharedScaleFixtureRepoPath (faif.4 shared cache, gated by
	// BWB_SCALE_FIXTURE=1) so the scale fixture is seeded once per process
	// rather than re-seeded per test.  BWB_SCALE_FIXTURE_SMOKE=1 is still
	// checked for backward compatibility, but the cache gate is BWB_SCALE_FIXTURE.
	checkScaleGateEnabled(t)                  // checks BWB_SCALE_FIXTURE_SMOKE=1
	repoPath := SharedScaleFixtureRepoPath(t) // seeds once, checks BWB_SCALE_FIXTURE=1
	repo := newScaleRepository(repoPath)
	ctx := context.Background()

	for _, kw := range []string{"workflow", "pipeline", "dashboard"} {
		kw := kw
		t.Run("keyword_"+kw, func(t *testing.T) {
			page, err := repo.Search(ctx, domain.SearchIssuesQuery{
				Text: kw,
			})
			if err != nil {
				t.Fatalf("Search(%q): %v", kw, err)
			}
			if len(page.Results) < 20 {
				t.Errorf("Search(%q): got %d results; want >=20", kw, len(page.Results))
			}
		})
	}
}

// TestScaleFixtureRepository_ShowIssueEdgeCases guards the repository's ability to
// handle edge-case issue titles (emoji, shell metacharacters, max-length) and
// the null-description issue (781a regression guard).
func TestScaleFixtureRepository_ShowIssueEdgeCases(t *testing.T) {
	// regression class: 781a (null description), repository edge-case resilience
	//
	// Uses SharedScaleFixtureRepoPath (faif.4 shared cache) — see
	// TestScaleFixtureRepository_SearchKeywordReturnsGe20Results for rationale.
	checkScaleGateEnabled(t) // checks BWB_SCALE_FIXTURE_SMOKE=1
	spec := loadScaleSeed(t)
	repoPath := SharedScaleFixtureRepoPath(t) // seeds once, checks BWB_SCALE_FIXTURE=1
	repo := newScaleRepository(repoPath)
	ctx := context.Background()

	// Locate edge-case issue IDs from the spec.
	var (
		emojiID    string
		metacharID string
		maxlenID   string
		nullDescID string
	)
	for _, iss := range spec.Issues {
		switch {
		case containsRune(iss.Title, '🚀') && emojiID == "":
			emojiID = iss.ID
		case isShellMetacharTitle(iss.Title) && metacharID == "":
			metacharID = iss.ID
		case len(iss.Title) >= 120 && maxlenID == "":
			maxlenID = iss.ID
		case iss.Description == "" && nullDescID == "":
			nullDescID = iss.ID
		}
	}

	cases := []struct {
		name    string
		id      string
		comment string
	}{
		{"emoji_title", emojiID, "emoji title (edge-case text encoding)"},
		{"shell_metachar_title", metacharID, "shell metacharacter title (injection guard)"},
		{"max_length_title", maxlenID, "max-length title (overflow guard)"},
		{"null_description", nullDescID, "781a regression guard: missing description field"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.id == "" {
				t.Fatalf("ShowIssue %s: could not find matching issue in scale spec", tc.comment)
			}
			detail, err := repo.Issue(ctx, tc.id)
			if err != nil {
				t.Errorf("ShowIssue(%q) [%s]: unexpected error: %v", tc.id, tc.comment, err)
				return
			}
			if detail.Summary.ID == "" {
				t.Errorf("ShowIssue(%q) [%s]: returned empty ID", tc.id, tc.comment)
			}
		})
	}
}

// containsRune reports whether s contains the given rune.
func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}

// isShellMetacharTitle reports whether the title contains shell metacharacters
// (backtick, semicolon, single quote, double quote).
func isShellMetacharTitle(title string) bool {
	for _, r := range []rune{'`', ';', '\'', '"'} {
		if containsRune(title, r) {
			return true
		}
	}
	return false
}
