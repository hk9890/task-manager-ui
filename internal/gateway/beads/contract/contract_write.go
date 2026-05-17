// Package contract — write-contract suite.
//
// RunWriteContract is the write-side analog of RunReadContract. It encodes
// the postconditions from the write-method documentation in interface.go as
// parameterised, fixture-agnostic invariant assertions.
//
// Each sub-test is isolated: it creates its own issue(s) via the gateway so
// there is no state shared between sub-tests. This means RunWriteContract
// requires a fresh, writable database per invocation (not the shared read-only
// fixture used by RunReadContract).
//
// Sub-tests run sequentially (no t.Parallel). bd 1.0.4 makes real-bd
// subprocess setup serial regardless (every test does `bd init` + several
// writes), so the test-level parallelism wins very little and the
// fixture-pool engineering would be complex. Sequential keeps the suite
// straightforward to reason about.
//
// Wire RunWriteContract against:
//   - FakeBeadsGateway (unit tier) via fake_write_contract_test.go
//   - Real CLI gateway with a per-test mktemp DB (integration tier) via
//     real_write_contract_integration_test.go
package contract

import (
	"context"
	"errors"
	"testing"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/gateway/beads"
	"github.com/hk9890/beads-workbench/internal/gateway/beads/contractcheck"
)

// WritableGatewayFactory returns a BeadsGateway bound to a fresh, writable DB.
// Unlike GatewayFactory (used by RunReadContract), the returned gateway must
// support write operations. The factory is called once per RunWriteContract
// invocation; cleanup is registered via t.Cleanup.
//
// For real-bd tests this means calling WritableTempFixture(t) from the
// datasets package (mktemp + bd init, unique per test). For fake tests,
// a freshly-constructed FakeBeadsGateway with write support is sufficient.
type WritableGatewayFactory func(t *testing.T) beads.BeadsGateway

// RunWriteContract runs one sub-test per write method on BeadsGateway, plus
// cross-method observability invariants. The factory must provide a writable
// gateway with no pre-existing issues (empty DB is fine; the suite creates
// everything it needs).
//
// Invariants encoded:
//
//	CreateIssue/HappyPath           — non-empty ID returned; ShowIssue finds it.
//	CreateIssue/RequiredFields      — empty title → ErrorCodeCommandFailed.
//	UpdateIssue/HappyPath           — post-Update, ShowIssue reflects change.
//	UpdateIssue/NonExistent         — unknown ID → ErrorCodeCommandFailed.
//	CloseIssue/HappyPath            — post-Close, ShowIssue.Status=="closed".
//	CloseIssue/Idempotency          — re-closing exits 0 (no error).
//	AddComment/HappyPath            — post-AddComment, ShowIssue includes comment.
//	WriteVisibilityInvariant        — every successful write visible via read.
//	CountIncrementInvariant         — CreateIssue(status=open) increments count.
func RunWriteContract(t *testing.T, factory WritableGatewayFactory) {
	t.Helper()

	ctx := context.Background()

	// ---- CreateIssue ----

	t.Run("CreateIssue/HappyPath", func(t *testing.T) {
		gw := factory(t)

		result, err := gw.CreateIssue(ctx, domain.CreateIssueInput{
			Title:       "Contract test issue",
			Description: "Created by RunWriteContract CreateIssue/HappyPath",
			Type:        "task",
		})
		if err != nil {
			t.Fatalf("CreateIssue: unexpected error: %v", err)
		}

		// Validate result structure.
		assertNoViolations(t, contractcheck.ValidateCreateIssueResult("CreateIssue", result))

		// Cross-method: ShowIssue(returnedID) must find the issue.
		newID := result.IssueID
		detail, err := gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: newID})
		if err != nil {
			t.Fatalf("ShowIssue(%q) after CreateIssue: unexpected error: %v", newID, err)
		}

		// Returned ID must match what we asked for.
		if detail.Summary.ID != newID {
			t.Errorf("ShowIssue after CreateIssue: Summary.ID=%q, want %q", detail.Summary.ID, newID)
		}

		// Title must round-trip.
		assertNoViolations(t, contractcheck.ValidateWriteVisibility(
			"CreateIssue/HappyPath", "TitleRoundTrip", detail, "Contract test issue",
		))
	})

	t.Run("CreateIssue/RequiredFields", func(t *testing.T) {
		gw := factory(t)

		// Empty title — bd exits 1 with {"error":"title required..."}.
		// The contract documents ErrorCodeCommandFailed for this case.
		_, err := gw.CreateIssue(ctx, domain.CreateIssueInput{Title: ""})
		if err == nil {
			t.Fatal("CreateIssue(empty title): expected error, got nil")
		}

		var gwErr domain.GatewayError
		if errors.As(err, &gwErr) {
			if gwErr.Code != domain.ErrorCodeCommandFailed {
				t.Errorf("CreateIssue(empty title): expected ErrorCodeCommandFailed, got %q", gwErr.Code)
			}
		} else {
			t.Errorf("CreateIssue(empty title): expected domain.GatewayError, got %T: %v", err, err)
		}
	})

	// ---- UpdateIssue ----

	t.Run("UpdateIssue/HappyPath", func(t *testing.T) {
		gw := factory(t)

		// Setup: create an issue to update.
		createResult, err := gw.CreateIssue(ctx, domain.CreateIssueInput{
			Title: "Issue to update",
			Type:  "task",
		})
		if err != nil {
			t.Fatalf("CreateIssue setup: unexpected error: %v", err)
		}

		newTitle := "Updated title by RunWriteContract"
		if err := gw.UpdateIssue(ctx, createResult.IssueID, domain.UpdateIssueInput{
			Title: &newTitle,
		}); err != nil {
			t.Fatalf("UpdateIssue: unexpected error: %v", err)
		}

		// Cross-method: ShowIssue must reflect the updated title.
		updatedID := createResult.IssueID
		detail, err := gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: updatedID})
		if err != nil {
			t.Fatalf("ShowIssue after UpdateIssue: unexpected error: %v", err)
		}

		assertNoViolations(t, contractcheck.ValidateWriteVisibility(
			"UpdateIssue/HappyPath", "TitleRoundTrip", detail, newTitle,
		))
	})

	t.Run("UpdateIssue/NonExistent", func(t *testing.T) {
		gw := factory(t)

		// The contract documents ErrorCodeCommandFailed for an unknown issueID.
		title := "ghost update"
		err := gw.UpdateIssue(ctx, "nonexistent-zzz-9999", domain.UpdateIssueInput{
			Title: &title,
		})
		if err == nil {
			t.Fatal("UpdateIssue(nonexistent): expected error, got nil")
		}

		var gwErr domain.GatewayError
		if errors.As(err, &gwErr) {
			if gwErr.Code != domain.ErrorCodeCommandFailed {
				t.Errorf("UpdateIssue(nonexistent): expected ErrorCodeCommandFailed, got %q", gwErr.Code)
			}
		} else {
			t.Errorf("UpdateIssue(nonexistent): expected domain.GatewayError, got %T: %v", err, err)
		}
	})

	// ---- CloseIssue ----

	t.Run("CloseIssue/HappyPath", func(t *testing.T) {
		gw := factory(t)

		createResult, err := gw.CreateIssue(ctx, domain.CreateIssueInput{
			Title: "Issue to close",
			Type:  "task",
		})
		if err != nil {
			t.Fatalf("CreateIssue setup: unexpected error: %v", err)
		}

		if err := gw.CloseIssue(ctx, createResult.IssueID, domain.CloseIssueInput{
			Reason: "done",
		}); err != nil {
			t.Fatalf("CloseIssue: unexpected error: %v", err)
		}

		// Cross-method: ShowIssue must show status="closed".
		closedID := createResult.IssueID
		detail, err := gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: closedID})
		if err != nil {
			t.Fatalf("ShowIssue after CloseIssue: unexpected error: %v", err)
		}

		assertNoViolations(t, contractcheck.ValidateWriteVisibility(
			"CloseIssue/HappyPath", "StatusAfterClose", detail, "",
		))
	})

	t.Run("CloseIssue/Idempotency", func(t *testing.T) {
		gw := factory(t)

		createResult, err := gw.CreateIssue(ctx, domain.CreateIssueInput{
			Title: "Issue to close twice",
			Type:  "task",
		})
		if err != nil {
			t.Fatalf("CreateIssue setup: unexpected error: %v", err)
		}

		// First close.
		if err := gw.CloseIssue(ctx, createResult.IssueID, domain.CloseIssueInput{}); err != nil {
			t.Fatalf("CloseIssue (first): unexpected error: %v", err)
		}

		// Second close — must be idempotent. The gateway emulates idempotency
		// over bd 1.0.4's known lookup bug (re-close within ~1s returns
		// "issue not found" because bd close's ID lookup uses the default
		// status filter that excludes already-closed issues). See the
		// CloseIssue note in interface.go for the bd characterization.
		if err := gw.CloseIssue(ctx, createResult.IssueID, domain.CloseIssueInput{}); err != nil {
			t.Errorf("CloseIssue (second, idempotency): expected nil error, got %v", err)
		}
	})

	// ---- AddComment ----

	t.Run("AddComment/HappyPath", func(t *testing.T) {
		gw := factory(t)

		createResult, err := gw.CreateIssue(ctx, domain.CreateIssueInput{
			Title: "Issue for comment",
			Type:  "task",
		})
		if err != nil {
			t.Fatalf("CreateIssue setup: unexpected error: %v", err)
		}

		commentBody := "RunWriteContract AddComment/HappyPath verification comment"
		if err := gw.AddComment(ctx, createResult.IssueID, domain.AddCommentInput{
			Body: commentBody,
		}); err != nil {
			t.Fatalf("AddComment: unexpected error: %v", err)
		}

		// Cross-method: ShowIssue must include the new comment.
		commentIssueID := createResult.IssueID
		detail, err := gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: commentIssueID})
		if err != nil {
			t.Fatalf("ShowIssue after AddComment: unexpected error: %v", err)
		}

		assertNoViolations(t, contractcheck.ValidateWriteVisibility(
			"AddComment/HappyPath", "CommentVisible", detail, commentBody,
		))
	})

	// =========================================================
	// Cross-method invariants
	// =========================================================

	// WriteVisibilityInvariant: every successful write must be visible via the
	// subsequent ShowIssue call. Covers all four write methods in one sub-test.

	t.Run("WriteVisibilityInvariant", func(t *testing.T) {
		gw := factory(t)

		// --- Create → Show ---
		createResult, err := gw.CreateIssue(ctx, domain.CreateIssueInput{
			Title: "visibility-invariant-issue",
			Type:  "task",
		})
		if err != nil {
			t.Fatalf("CreateIssue: unexpected error: %v", err)
		}
		visID := createResult.IssueID

		detail, err := gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: visID})
		if err != nil {
			t.Fatalf("ShowIssue after CreateIssue: unexpected error: %v", err)
		}
		assertNoViolations(t, contractcheck.ValidateWriteVisibility(
			"WriteVisibilityInvariant/Create→Show", "TitleRoundTrip", detail, "visibility-invariant-issue",
		))

		// --- Update → Show ---
		newTitle := "visibility-invariant-issue-updated"
		if err := gw.UpdateIssue(ctx, visID, domain.UpdateIssueInput{
			Title: &newTitle,
		}); err != nil {
			t.Fatalf("UpdateIssue: unexpected error: %v", err)
		}

		detail, err = gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: visID})
		if err != nil {
			t.Fatalf("ShowIssue after UpdateIssue: unexpected error: %v", err)
		}
		assertNoViolations(t, contractcheck.ValidateWriteVisibility(
			"WriteVisibilityInvariant/Update→Show", "TitleRoundTrip", detail, newTitle,
		))

		// --- AddComment → Show ---
		commentBody := "WriteVisibilityInvariant comment body"
		if err := gw.AddComment(ctx, visID, domain.AddCommentInput{
			Body: commentBody,
		}); err != nil {
			t.Fatalf("AddComment: unexpected error: %v", err)
		}

		detail, err = gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: visID})
		if err != nil {
			t.Fatalf("ShowIssue after AddComment: unexpected error: %v", err)
		}
		assertNoViolations(t, contractcheck.ValidateWriteVisibility(
			"WriteVisibilityInvariant/AddComment→Show", "CommentVisible", detail, commentBody,
		))

		// --- Close → Show ---
		if err := gw.CloseIssue(ctx, visID, domain.CloseIssueInput{}); err != nil {
			t.Fatalf("CloseIssue: unexpected error: %v", err)
		}

		detail, err = gw.ShowIssue(ctx, domain.ShowIssueQuery{IssueID: visID})
		if err != nil {
			t.Fatalf("ShowIssue after CloseIssue: unexpected error: %v", err)
		}
		assertNoViolations(t, contractcheck.ValidateWriteVisibility(
			"WriteVisibilityInvariant/Close→Show", "StatusAfterClose", detail, "",
		))
	})

	// WriteVisibilityInvariant/DeliberateBreak: demonstrates the assertion has
	// teeth. A mock inner that lies about ShowIssue results is detected.

	t.Run("WriteVisibilityInvariant/DeliberateBreak", func(t *testing.T) {
		// Use a mockViolatingDetail that returns a stale title — simulating a
		// gateway that does not persist writes between calls.
		brokenDetail := domain.IssueDetail{
			Summary: domain.IssueSummary{
				ID:     "fake-id",
				Title:  "stale title", // intentionally wrong
				Status: "open",        // intentionally not "closed"
			},
			Comments: []domain.IssueComment{},
		}

		// TitleRoundTrip violation: written "expected title", read back "stale title".
		violations := contractcheck.ValidateWriteVisibility(
			"WriteVisibilityInvariant/DeliberateBreak/TitleRoundTrip",
			"TitleRoundTrip",
			brokenDetail,
			"expected title",
		)
		if len(violations) == 0 {
			t.Error("DeliberateBreak/TitleRoundTrip: expected violation, got none (assertion has no teeth)")
		}

		// StatusAfterClose violation: status is "open", not "closed".
		violations = contractcheck.ValidateWriteVisibility(
			"WriteVisibilityInvariant/DeliberateBreak/StatusAfterClose",
			"StatusAfterClose",
			brokenDetail,
			"",
		)
		if len(violations) == 0 {
			t.Error("DeliberateBreak/StatusAfterClose: expected violation, got none (assertion has no teeth)")
		}

		// CommentVisible violation: comments slice is empty, body is absent.
		violations = contractcheck.ValidateWriteVisibility(
			"WriteVisibilityInvariant/DeliberateBreak/CommentVisible",
			"CommentVisible",
			brokenDetail,
			"missing comment",
		)
		if len(violations) == 0 {
			t.Error("DeliberateBreak/CommentVisible: expected violation, got none (assertion has no teeth)")
		}
	})

	// CountIncrementInvariant: after CreateIssue(status=open), CountIssues(open)
	// total must increase by exactly 1.

	t.Run("CountIncrementInvariant", func(t *testing.T) {
		gw := factory(t)

		// Count before.
		beforeResult, err := gw.CountIssues(ctx, domain.IssueCountQuery{Statuses: []string{"open"}})
		if err != nil {
			t.Fatalf("CountIssues before CreateIssue: unexpected error: %v", err)
		}
		countBefore := beforeResult.Total

		// Create an open issue (no status override → bd defaults to "open").
		_, err = gw.CreateIssue(ctx, domain.CreateIssueInput{
			Title: "count-increment-invariant-issue",
			Type:  "task",
		})
		if err != nil {
			t.Fatalf("CreateIssue: unexpected error: %v", err)
		}

		// Count after.
		afterResult, err := gw.CountIssues(ctx, domain.IssueCountQuery{Statuses: []string{"open"}})
		if err != nil {
			t.Fatalf("CountIssues after CreateIssue: unexpected error: %v", err)
		}
		countAfter := afterResult.Total

		assertNoViolations(t, contractcheck.ValidateCountIncrement(
			"CountIncrementInvariant", "open", countBefore, countAfter,
		))
	})

	// CountIncrementInvariant/DeliberateBreak: demonstrates that the count
	// assertion would catch a gateway that does not persist the created issue.

	t.Run("CountIncrementInvariant/DeliberateBreak", func(t *testing.T) {
		// Simulate: create added issue but count stayed the same (bad gateway).
		countBefore := 5
		countAfterBroken := 5 // did NOT increase

		violations := contractcheck.ValidateCountIncrement(
			"CountIncrementInvariant/DeliberateBreak", "open", countBefore, countAfterBroken,
		)
		if len(violations) == 0 {
			t.Error("DeliberateBreak: expected violation when count did not increase, got none (assertion has no teeth)")
		}
	})
}
