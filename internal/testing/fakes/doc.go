// Package fakes provides contract-conforming fake implementations of
// external dependencies used in tests across this module.
//
// # FakeBeadsGateway — a contract-conforming peer of the real gateway
//
// FakeBeadsGateway is NOT a casual mock. It is a peer implementation of
// beads.BeadsGateway that must pass the SAME RunReadContract test suite
// (internal/gateway/beads/contract/) as the real bd-backed gateway. When
// its behavior diverges from real bd, tests that rely on it become a lie:
// they pass against the fake but the production code would break against
// real bd. The fixed-this-week bug class (ssom, kh54, o7tk) all involved
// exactly this divergence pattern.
//
// # Discipline: when a bug's root cause involves fake-vs-real divergence
//
// The fix MUST include:
//
//  1. Update the fake to match real bd's observed behavior. The contract
//     audit in interface.go documents what real bd does for every method;
//     the fake should produce values that satisfy the same invariants
//     defined in contractcheck/contractcheck.go.
//
//  2. Add or strengthen an invariant in
//     internal/gateway/beads/contract/contract.go (or its delegated
//     validators in internal/gateway/beads/contractcheck/) so that
//     RunReadContract catches the divergence on the next run.
//
//  3. Record the change in the Fake drift log below.
//
// # Fake drift log
//
// 2026-05-16: ShowIssue helpers initialized Comments/BlockedBy as nil for
//
//	non-blocked issues; real bd emits empty arrays as []. Updated
//	primeFakeFromFixtureSpec (fake_contract_test.go) to seed
//	Comments: []domain.IssueComment{} and BlockedBy: []domain.IssueReference{}
//	on every IssueDetail entry that has no explicit value.
//	Discovered: 8qw9.3 fake fidelity audit; surfaced by
//	Invariants/ShowIssue/CommentsNotNil and Invariants/ShowIssue/BlockedByNotNil.
//
// 2026-05-16: Query() returned QueryResponse verbatim regardless of expr; real
//
//	bd filters by expression. Added QueryResponsesByExpr map keyed by
//	expression string. When set, Query looks up expr in the map and returns
//	the matching slice; falls back to QueryResponse when the key is absent.
//	Discovered: 8qw9.3; surfaced by Invariants/Query/StatusFilterRespected.
//
// 2026-05-17: CreateIssue() returned CreateIssueResponse verbatim (empty ID,
//
//	no stored state); real bd generates a unique ID and the issue is
//	retrievable via ShowIssue. Added in-memory write-state store (issueStore)
//	initialised in NewFakeBeadsGateway. CreateIssue now validates empty title
//	→ ErrorCodeCommandFailed, generates "tmp-<n>" IDs, and stores the issue.
//	ShowIssue reads from the store first (then ShowIssuesByID, then
//	ShowIssueResponse). SeedIssue() helper added for tests that need
//	pre-existing issues without calling CreateIssue first.
//	Discovered: 9x70.3; surfaced by CreateIssue/RequiredFields and
//	CreateIssue/HappyPath in RunWriteContract.
//
// 2026-05-17: UpdateIssue() silently succeeded for unknown IDs; real bd exits
//
//	non-zero when the issueID cannot be resolved. UpdateIssue now checks
//	the write-state store and returns ErrorCodeCommandFailed for absent IDs.
//	Title/Description/Status mutations are reflected in the store and visible
//	via ShowIssue. Discovered: 9x70.3; surfaced by UpdateIssue/NonExistent
//	and UpdateIssue/HappyPath in RunWriteContract.
//
// 2026-05-17: CloseIssue() was a no-op; real bd sets the issue status to
//
//	"closed" and the change is visible via ShowIssue. CloseIssue now sets
//	Status="closed" in the write-state store (idempotent). Returns
//	ErrorCodeCommandFailed for unknown IDs.
//	Discovered: 9x70.3; surfaced by CloseIssue/HappyPath in RunWriteContract.
//
// 2026-05-17: AddComment() was a no-op; real bd appends a comment visible via
//
//	ShowIssue. AddComment now appends to the Comments slice in the write-state
//	store. Returns ErrorCodeCommandFailed for unknown IDs.
//	Discovered: 9x70.3; surfaced by AddComment/HappyPath in RunWriteContract.
//
// 2026-05-17: CountIssues() returned CountIssuesResponse verbatim; that static
//
//	stub does not reflect CreateIssue calls. CountIssues now counts live from
//	the write-state store when the store is non-empty, satisfying
//	CountIncrementInvariant. Falls back to CountIssuesResponse when the store
//	is empty (preserves existing UI-test stubs that never call CreateIssue).
//	Discovered: 9x70.3; surfaced by CountIncrementInvariant in RunWriteContract.
//
// 2026-05-18: SearchIssues() cache miss (text key absent from SearchResultsByText)
//
//	returned Results: nil; real bd always returns a non-nil slice (empty []
//	when no results). Fixed to return Results: []domain.SearchResult{}.
//	Discovered: beads-workbench-ix3j; parent epic beads-workbench-yspw.
//
// 2026-05-18: CloseIssue() did not set CloseReason on the stored IssueDetail;
//
//	real bd sets the close reason field. Fixed: when input.Reason is non-empty
//	it is used; otherwise the sentinel "Closed" is stored.
//	Discovered: beads-workbench-eifk; parent epic beads-workbench-yspw.
//
// 2026-05-18: CloseIssue() did not set ClosedAt on the stored IssueDetail;
//
//	real bd records a close timestamp. Fixed: existing.ClosedAt = time.Now().UTC()
//	is applied on every close (including idempotent re-close).
//	Discovered: beads-workbench-jsxd; parent epic beads-workbench-yspw.
//
// 2026-05-18: AddComment() constructed IssueComment{Body: ...} without an Author;
//
//	real bd records the comment author. Fixed: Author is set to the "fake-user"
//	sentinel, consistent with the fake's identity conventions.
//	Discovered: beads-workbench-n63l; parent epic beads-workbench-yspw.
//
// 2026-05-18: Query() accepted an empty expression and returned the verbatim
//
//	QueryResponse stub; real bd rejects an empty expression with a validation
//	error. Fixed: added a TrimSpace guard that returns GatewayError{Code:
//	ErrorCodeValidationFailed} before any store lookup.
//	Discovered: beads-workbench-tkgq; parent epic beads-workbench-yspw.
//
// 2026-05-18: LabelCatalog() returned nil when LabelCatalogResponse was unset;
//
//	real bd always returns a non-nil slice. Fixed: added a nil-check that
//	returns []domain.LabelOption{} when LabelCatalogResponse is nil.
//	Discovered: beads-workbench-fy7x; parent epic beads-workbench-yspw.
//
// 2026-05-18: CreateIssue() did not store input.Labels in the issueStore entry;
//
//	real bd persists labels and ShowIssue reflects them. Fixed: the labels slice
//	is now copied into Summary.Labels on the stored IssueDetail.
//	UpdateIssue() did not model Labels or ClearLabels mutations; real bd applies
//	--set-labels and --remove-label. Fixed: when input.Labels is non-empty the
//	stored Summary.Labels is replaced; when input.ClearLabels is true it is set
//	to an empty slice ([]string{}).
//	Discovered: beads-workbench-5toe; parent epic beads-workbench-yspw.
//	Surfaced by UpdateIssue/ClearLabels in RunWriteContract.
//
// # Sort-direction note
//
// FakeBeadsGateway.ListIssues and FakeBeadsGateway.Query do NOT simulate
// sort-direction. Both methods return results in insertion/seed order regardless
// of any SortField or SortDirection options present on the query. Tests that need
// to assert sort behaviour must use the real gateway (integration tier).
//
// # See also
//
//   - internal/gateway/beads/interface.go — canonical contract spec
//   - internal/gateway/beads/contract/contract.go — RunReadContract suite
//   - internal/gateway/beads/contractcheck/ — pure validators shared by
//     contract tests and the validatingGateway runtime decorator
//   - internal/gateway/beads/doc.go (Argv contract testing section) — the
//     outgoing-contract twin: pins the exact argv bwb sends to bd, ensuring
//     the gateway calls the right verb with the right flags. RecordingExecutor
//     in this package is the public executor used in those tests.
package fakes
