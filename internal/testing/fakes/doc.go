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
// # See also
//
//   - internal/gateway/beads/interface.go — canonical contract spec
//   - internal/gateway/beads/contract/contract.go — RunReadContract suite
//   - internal/gateway/beads/contractcheck/ — pure validators shared by
//     contract tests and the validatingGateway runtime decorator
package fakes
