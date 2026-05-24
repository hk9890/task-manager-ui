//go:build integration

package contract_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
	bdrunner "github.com/hk9890/beads-workbench/internal/gateway/beads"
	beads "github.com/hk9890/beads-workbench/internal/repository/beads"
	"github.com/hk9890/beads-workbench/internal/testing/e2e/embeddedfixture"
)

// runBD executes a bd command in the given repo directory and returns an error
// on non-zero exit. It sets BD_NON_INTERACTIVE=1 to suppress interactive prompts.
func runBD(repoPath string, args ...string) error {
	cmd := exec.Command("bd", args...)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd %s failed: %w\n%s", strings.Join(args, " "), err, out)
	}
	return nil
}

// newFixtureGateway returns a per-test copy of the shared pre-seeded fixture and
// a CLI gateway bound to it. It uses SharedFixtureRepoPath so the expensive seed
// step runs only once per process; subsequent calls do a fast directory copy.
func newFixtureGateway(t *testing.T) (beads.BeadsGateway, string) {
	t.Helper()
	repoPath := embeddedfixture.SharedFixtureRepoPath(t)
	runner := bdrunner.NewCommandRunner(bdrunner.RunnerConfig{
		WorkDir: repoPath,
	})
	return beads.NewCLIGateway(runner), repoPath
}

// TestRealGatewayLinksAndDepsScenario exercises bd link, bd dep relate, and
// bd dep add as subprocess test setup, then verifies each mutation via the
// gateway's ShowIssue and BlockedIssues read methods.
//
// Each step uses its own fresh fixture to avoid state bleed between setup
// operations. Total wall time target is <15s.
func TestRealGatewayLinksAndDepsScenario(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not found on PATH; skipping links and deps scenario test")
	}

	t.Setenv("BEADS_ACTOR", "fixture-user")

	ctx := context.Background()

	// ---- Step 1: bd link --type related ----
	//
	// bd link bwf-2 bwf-3 --type related creates a bidirectional related link.
	// ShowIssue("bwf-2").Related must contain bwf-3.

	t.Run("BDLinkRelated", func(t *testing.T) {
		gw, repoPath := newFixtureGateway(t)

		if err := runBD(repoPath, "link", "bwf-2", "bwf-3", "--type", "related"); err != nil {
			t.Fatalf("step 1: bd link bwf-2 bwf-3 --type related: %v", err)
		}

		detail, err := gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: "bwf-2"})
		if err != nil {
			t.Fatalf("step 1: ShowIssue(bwf-2): unexpected error: %v", err)
		}

		if detail.Summary.ID != "bwf-2" {
			t.Errorf("step 1: ShowIssue: ID: got %q, want %q", detail.Summary.ID, "bwf-2")
		}

		found := false
		for _, ref := range detail.Related {
			if ref.ID == "bwf-3" {
				found = true
				break
			}
		}
		if !found {
			relatedIDs := make([]string, len(detail.Related))
			for i, r := range detail.Related {
				relatedIDs[i] = r.ID
			}
			t.Errorf("step 1: ShowIssue(bwf-2).Related: expected bwf-3, got %v", relatedIDs)
		}
	})

	// ---- Step 2: bd dep relate ----
	//
	// bd dep relate bwf-3 bwf-2 creates a bidirectional relates-to dependency
	// (dependency_type="relates-to"). The gateway must map this to Related, not
	// to Blocks, for bwf-3.

	t.Run("BDDepRelate", func(t *testing.T) {
		gw, repoPath := newFixtureGateway(t)

		if err := runBD(repoPath, "dep", "relate", "bwf-3", "bwf-2"); err != nil {
			t.Fatalf("step 2: bd dep relate bwf-3 bwf-2: %v", err)
		}

		detail, err := gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: "bwf-3"})
		if err != nil {
			t.Fatalf("step 2: ShowIssue(bwf-3): unexpected error: %v", err)
		}

		if detail.Summary.ID != "bwf-3" {
			t.Errorf("step 2: ShowIssue: ID: got %q, want %q", detail.Summary.ID, "bwf-3")
		}

		// bwf-2 must appear under Related (not Blocks) for bwf-3.
		foundRelated := false
		for _, ref := range detail.Related {
			if ref.ID == "bwf-2" {
				foundRelated = true
				break
			}
		}
		if !foundRelated {
			relatedIDs := make([]string, len(detail.Related))
			for i, r := range detail.Related {
				relatedIDs[i] = r.ID
			}
			t.Errorf("step 2: ShowIssue(bwf-3).Related: expected bwf-2, got %v", relatedIDs)
		}

		// bwf-2 must NOT appear under Blocks for bwf-3.
		for _, ref := range detail.Blocks {
			if ref.ID == "bwf-2" {
				t.Errorf("step 2: ShowIssue(bwf-3).Blocks: bwf-2 must not appear under Blocks for a relates-to dependency, got Blocks=%v", detail.Blocks)
				break
			}
		}
	})

	// ---- Step 3: bd dep add ----
	//
	// The fixture seed already wires bwf-1 → bwf-2 via bd dep add (bwf-2 depends
	// on bwf-1). Running bd dep add bwf-2 bwf-1 again exercises the command path
	// (bd returns success idempotently). BlockedIssues must return bwf-2 with
	// bwf-1 as its blocker.
	//
	// Note: bd dep add <issue> <depends-on> means <issue> is blocked by <depends-on>.
	// "bwf-1 blocks bwf-2" is expressed as bd dep add bwf-2 bwf-1.

	t.Run("BDDepAdd", func(t *testing.T) {
		gw, repoPath := newFixtureGateway(t)

		// The seed already contains this dependency; running it again exercises
		// the bd dep add subprocess path and returns success idempotently.
		if err := runBD(repoPath, "dep", "add", "bwf-2", "bwf-1"); err != nil {
			t.Fatalf("step 3: bd dep add bwf-2 bwf-1: %v", err)
		}

		views, err := gw.BlockedIssues(ctx, domain.BlockedIssuesQuery{})
		if err != nil {
			t.Fatalf("step 3: BlockedIssues: unexpected error: %v", err)
		}

		var bwf2View *domain.BlockedIssueView
		for i := range views {
			if views[i].Issue.ID == "bwf-2" {
				bwf2View = &views[i]
				break
			}
		}
		if bwf2View == nil {
			blockedIDs := make([]string, len(views))
			for i, v := range views {
				blockedIDs[i] = v.Issue.ID
			}
			t.Fatalf("step 3: BlockedIssues: bwf-2 not found; got %v", blockedIDs)
		}

		if len(bwf2View.BlockedBy) == 0 {
			t.Fatal("step 3: BlockedIssues: bwf-2.BlockedBy must be non-empty")
		}

		foundBlocker := false
		for _, ref := range bwf2View.BlockedBy {
			if ref.ID == "bwf-1" {
				foundBlocker = true
				break
			}
		}
		if !foundBlocker {
			blockerIDs := make([]string, len(bwf2View.BlockedBy))
			for i, r := range bwf2View.BlockedBy {
				blockerIDs[i] = r.ID
			}
			t.Errorf("step 3: BlockedIssues: bwf-2.BlockedBy: expected bwf-1, got %v", blockerIDs)
		}
	})
}
