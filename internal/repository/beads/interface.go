// Package beads defines the BeadsGateway interface and its CLI-backed implementation.
//
// # Re-verification 2026-05-18 (epic yspw)
//
// All documented contract clauses across the 8 method groups were systematically
// re-audited against bd 1.0.4 and the test suites. The audit surfaced 23
// follow-up items (1 P1, 5 P2, 14 P3, 3 P4) — all closed. Notable outcomes:
// (a) py38 P1 production bug fixed (UpdateIssue ClearLabels workaround emitted
// `--remove-labels` plural; bd 1.0.4's actual flag is `--remove-label`
// singular), (b) puy3 P2 TypeCatalog decoder updated to accept `custom_types`
// as `[]string` (bd 1.0.4's actual shape), (c) uij1 ReadyIssues contract
// corrected (bd ready returns ONLY status=open issues — in_progress is
// excluded). Both upstream workarounds (CloseIssue idempotency over
// gastownhall/beads#4025; UpdateIssue --set-labels "" silent fail) remain
// necessary at bd 1.0.4. (Note: FakeBeadsGateway referenced in the original
// audit was removed in the 8pxi refactor.)
//
// # Observed bd quirks (audit 2026-05-16)
//
// Systemic observations from a hands-on audit of bd version 1.0.4 behavior against a
// freshly initialised temp DB at /tmp/tmp.dbLu1NDdcf (mktemp -d && bd init).
// Audit covered 5 issues across open/in_progress/blocked/closed statuses, with
// dependencies, labels, assignees, and varied priorities. bd version: 1.0.4.
//
// bd sort direction inverts relative to the naive intuition.
//   - bd --sort <field> defaults to DESCENDING order (newest/highest first).
//   - bd --reverse inverts to ASCENDING (oldest/lowest first).
//   - Disposition: ACCEPT. The gateway now correctly translates the domain
//     model: SortDirectionDescending emits NO flag (bd default = DESC),
//     SortDirectionAscending emits --reverse. Fixed in bug zhef (2026-05-16);
//     a pre-fix gateway version had this inverted and produced the wrong
//     order for every sorted query.
//
// bd search requires a non-empty query argument.
//   - bd search --json without a text arg exits non-zero with {"error":"search query is required"}.
//   - Disposition: CARVE OUT — SearchIssues routes empty-text queries through
//     bd list --all --json rather than bd search, so callers never see this error.
//
// bd search --status accepts only a single token, not comma-separated values.
//   - bd search text --status open,closed returns [] (empty), not the union.
//   - bd list --status open,closed correctly returns the union.
//   - Disposition: CARVE OUT — SearchIssues passes --status all for the multi-status
//     case when routing through bd search. Single-status filters work correctly.
//     Multi-status search with text is currently unimplemented at the bd layer.
//
// bd show omits the "description" field entirely (not null, not "") when unset.
//   - Disposition: CARVE OUT. The decoder uses optionalString(primary.Description)
//     so an absent description maps to IssueDetail.Description = "". Fixed in
//     bug 781a (2026-05-16); a pre-fix gateway version treated this as a required
//     field and returned ErrorCodeDecodeFailed for any issue without a description.
//
// bd blocked returns dependency-blocked issues, not status=blocked issues.
//   - "bd blocked --json" returns issues with unresolved dependency blockers,
//     regardless of their stored status field (which is typically "open").
//   - "bd list --status blocked" returns issues whose stored status IS "blocked"
//     (manually set, independent of any dependency graph).
//   - These two populations are distinct and may not overlap at all.
//   - Disposition: ACCEPT — the gateway exposes both via BlockedIssues (bd blocked)
//     and ListIssues(status=blocked). ReadyExplain.Blocked also uses the
//     dependency-blocked definition. Consumers must be aware of the distinction.
//
// bd statuses --json omits "custom_statuses" key when no custom statuses exist.
//   - The decoder handles this gracefully (nil slice).
//   - Disposition: ACCEPT — no consumer impact.
//
// bd count --by-status --json includes all statuses (including closed) by default.
//   - No --all flag needed for CountIssues to include closed groups.
//   - Disposition: ACCEPT — documented as a postcondition.
//
// Methodology: temp DB created with bd init; 5 issues created covering all
// key statuses and dependency relationships. bd list, bd ready, bd blocked,
// bd ready --explain, bd search, bd query, bd count, bd statuses, bd types,
// bd label list-all, bd show, and bd ping were each exercised with --json
// across: no filter, single filter, multi-filter, sort, limit, and edge cases.
// Replay: recreate by running `mktemp -d && cd <dir> && BD_NON_INTERACTIVE=1 bd init`
// and issuing the commands listed in task beads-workbench-8qw9.1 comment thread.
//
// # Observed bd write quirks (audit 2026-05-17)
//
// Hands-on audit of bd 1.0.4 write behavior against a freshly initialised temp DB
// at /tmp/tmp.8GmEk81tiS (mktemp -d && BD_NON_INTERACTIVE=1 bd init). All four write
// methods exercised: CreateIssue (10 variants), UpdateIssue (12 variants), CloseIssue
// (5 variants), AddComment (7 variants). Total: 34 distinct real-bd invocations, all
// issued with BD_NON_INTERACTIVE=1. Full evidence log: /tmp/9x70-audit.log. See task
// beads-workbench-9x70.1 for the invocation list and per-case outcomes.
//
// Write responses are NOT JSON by default.
//   - bd create returns a JSON payload ({"id":"...", ...}) on stdout when --json is passed.
//   - bd update returns the updated issue as a JSON array when --json is passed; without
//     --json it emits a human-readable "✓ Updated issue: <id> — <title>" message.
//   - bd close returns the closed issue as a JSON array when --json is passed; without
//     --json it emits "✓ Closed <id> — <title>: <reason>" on stdout.
//   - bd comments add always emits "Comment added to <id>" (no --json flag observed).
//   - The current gateway uses --json only for CreateIssue (to capture the new ID).
//     UpdateIssue, CloseIssue, and AddComment discard stdout. Disposition: ACCEPT.
//
// bd update --set-labels "" silently ignores the clear request (bd 1.0.4 bug).
//   - Passing --set-labels "" exits 0 with a success message but labels remain unchanged.
//   - Disposition: WORKAROUND LANDED (see [[ubav]], fixed in py38). The gateway's
//     ClearLabels path now pre-fetches the current labels via ShowIssue, then emits
//     `bd update --remove-label <csv>` (singular flag, per bd 1.0.4) enumerating each
//     existing label. If the issue has no labels, the bd update call is skipped entirely.
//     The upstream bd bug is still present; this workaround bypasses it at the
//     gateway layer.
//
// bd update with no field flags exits 0 with "No updates specified" on stdout.
//   - When UpdateIssueInput has all nil pointer fields and ClearLabels=false, the gateway
//     emits "bd update <id>" with no flags. bd exits 0. Gateway returns nil error.
//   - Disposition: ACCEPT — this path is unreachable via the app's edit modal, which
//     always populates at least one field before submitting.
//
// Actor attribution uses git user.name; --actor and BEADS_ACTOR can override it.
//   - bd create, bd update, and bd comments add support --actor "<name>" to set the
//     created_by / author field. Without --actor, bd uses the git user.name from the
//     repository's git config.
//   - BEADS_ACTOR env var also overrides the actor for bd create (bd 1.0.4 flag help:
//     `--actor default: $BEADS_ACTOR, git user.name, $USER`).
//   - The gateway never passes --actor and filterEnvToAllowlist strips BEADS_ACTOR
//     (it is not in the allowlist and lacks the BWB_ prefix). All writes are attributed
//     to the git user.name of the beads project's git config. Disposition: ACCEPT.
//
// CloseIssue is idempotent (the gateway emulates this over a bd 1.0.4 bug).
//   - When no --reason is supplied, bd stores close_reason as "Closed" (the literal
//     string), visible in ShowIssue. Disposition: ACCEPT.
//   - bd 1.0.4 bug (filed upstream as gastownhall/beads#4025):
//     `bd close <id>` infers "issue not found" from result.RowsAffected()==0
//     in internal/storage/issueops/close.go. The schema's `closed_at` and
//     `updated_at` columns are DATETIME (second resolution per
//     internal/storage/schema/migrations/0001_create_issues.up.sql:17-18),
//     so a re-close within the same wall-clock second changes NO columns
//     (status/closed_at/updated_at/close_reason/closed_by_session are all
//     already at their target values). MySQL/Dolt returns RowsAffected==0
//     and bd emits "Error closing <id>: issue not found: <id>" — but the
//     issue still exists with status=closed (verifiable via ShowIssue or
//     `bd list --status closed`). Decisive evidence: passing --reason
//     "different reason" on the second close forces close_reason to
//     change, RowsAffected==1, and the failure rate drops from ~40% to 0%.
//     Earlier "default filter excludes closed" hypothesis was WRONG — the
//     SQL is `WHERE id = ?` with no status filter; it's purely the
//     RowsAffected semantic gap.
//   - Gateway behavior (workaround — REMOVE WHEN UPSTREAM FIXED): see
//     CloseIssue in writes.go. Catches the close-specific "issue not found"
//     stderr, runs ShowIssue, returns nil iff the issue exists with
//     status=closed. Truly missing issues still surface the bd error.
//     Preserves the idempotency contract for all callers until bd ships
//     the fix and we bump the mise-pinned version.
//
// Writes are immediately consistent with subsequent reads (embedded dolt auto-commit).
//   - bd uses an embedded dolt backend in auto-commit mode for standalone repos. Each
//     write is committed before the command exits; a subsequent bd show / bd list in
//     any process sees the updated state immediately. Disposition: ACCEPT.
//
// # Observed bd quirks at scale (faif.5 audit 2026-05-17, ~590 issue corpus)
//
// bd ready --json default 100-item cap vs bd ready --explain --json (uncapped).
//   - bd ready --json without --limit has a server-side default of 100 items. At
//     scale (>100 ready issues) it silently returns only the first 100 even when
//     hundreds more exist; no error or warning is emitted to stdout or stderr.
//   - bd ready --explain --json ignores the default limit and returns ALL ready and
//     blocked issues regardless of corpus size. The explain payload was observed to
//     return all 507 ready issues in the ~590-issue scale fixture.
//   - Impact: ReadyExplain (used by the board model) is correct and complete.
//     ReadyIssues (uses bd ready --json without --limit) is silently capped at 100
//     on scale datasets. Callers using ReadyIssues on large repos will see at most
//     100 ready issues — the remainder are invisible.
//   - Parity test fix: BdReady calls in count/sort parity tests must pass --limit 0
//     to match ReadyExplain's uncapped output; omitting it causes a false count
//     mismatch (bwb_total=507 bd_count=100 delta=407 on the scale fixture).
//   - Disposition: ACCEPT for ReadyExplain (correct). KNOWN LIMITATION for
//     ReadyIssues — not used by the board model's hot path; acceptable for now.
package beads

import (
	"context"

	"github.com/hk9890/beads-workbench/internal/domain"
)

// BeadsGateway is the source-specific beads gateway used by the Beads Workbench UI.
// A gateway instance is bound to one beads source/project.
//
// All read methods on BeadsGateway are safe for concurrent use.
// Write methods (CreateIssue, UpdateIssue, CloseIssue, AddComment) acquire an
// exclusive runMu lock (CommandRunner.Run with IsWrite=true) and therefore
// serialize against all concurrent reads and other writes.
type BeadsGateway interface {
	// HealthCheck verifies that the bd CLI is reachable and a beads database
	// exists in the working directory.
	//
	// Preconditions:
	//   - ctx must be non-nil.
	//
	// Postconditions:
	//   - Returns nil when bd is reachable and the DB is healthy.
	//
	// Error semantics:
	//   - ErrorCodeCommandUnavailable: bd binary is not installed or not in PATH.
	//   - ErrorCodeNoDatabaseFound: bd is reachable but no .beads DB is present
	//     in the working directory tree.
	//   - ErrorCodeCommandFailed: bd ping exited non-zero for any other reason.
	//
	// bd quirks observed (audit 2026-05-16):
	//   - bd ping --json emits {"status":"ok","query_ms":N,...} — only "status"
	//     is meaningful for health; gateway ignores the payload and returns nil
	//     on zero exit. Disposition: ACCEPT.
	HealthCheck(ctx context.Context) error

	// ListIssues returns issue summaries matching the query using `bd list --json`.
	//
	// Preconditions:
	//   - ctx must be non-nil.
	//   - query.Limit, if > 0, caps the number of returned issues; Offset slices
	//     the capped window (gateway fetches Limit+Offset from bd, then slices).
	//
	// Postconditions:
	//   - Each returned IssueSummary has a non-empty ID, non-empty Title, non-empty
	//     Status, and non-empty Type.
	//   - By default (no Statuses filter), closed issues are excluded; only open,
	//     in_progress, blocked, deferred, and pinned issues are returned. Pass
	//     Statuses: []string{"closed"} or Statuses: []string{"all"} to include closed.
	//   - When query.Statuses is non-empty, bd receives --status <csv>. bd list
	//     accepts comma-separated values (e.g. "open,in_progress").
	//   - When query.SortBy is set:
	//     - bd sorts DESCENDING by default for all sort fields.
	//     - The gateway emits --reverse ONLY when SortOrder == SortDirectionAscending
	//       (so bd's default DESC becomes ASC). SortDirectionDescending emits no
	//       sort-direction flag and gets bd's default DESC behavior. The translation
	//       was inverted in a pre-zhef-fix gateway; see package-level quirks.
	//   - When query.Limit > 0 and bd returns more items than limit+offset, bd
	//     emits a warning to stderr ("Showing N issues; more results matched…").
	//     This warning does not appear in stdout and does not affect JSON parsing.
	//     The caller cannot tell from the returned slice alone whether more results
	//     exist; use CountIssues to disambiguate.
	//   - Assignee is populated from the "assignee" field when present; falls back
	//     to "owner" when "assignee" is absent.
	//   - Labels may be nil (not an empty slice) for issues with no labels.
	//
	// Error semantics:
	//   - ErrorCodeCommandFailed: bd exited non-zero (e.g. invalid status name).
	//   - ErrorCodeDecodeFailed: bd stdout was not parseable as JSON issue array.
	//
	// bd quirks observed (audit 2026-05-16):
	//   - bd --sort <field> defaults to descending; --reverse inverts to ascending.
	//     The gateway emits --reverse only for SortDirectionAscending. Disposition:
	//     ACCEPT (fixed in zhef 2026-05-16; was buggy in a pre-fix version).
	//   - bd list --json does NOT populate "dependencies", "dependents", "related",
	//     "description", "notes", "comments" for list results (those fields require
	//     bd show). The decoder's bdIssuePayload.Dependencies field is only populated
	//     for bd show output. Disposition: ACCEPT — list consumers only use summary
	//     fields; ShowIssue is the path to full detail.
	ListIssues(ctx context.Context, query domain.IssueListQuery) ([]domain.IssueSummary, error)

	// ReadyIssues returns issues that have no unresolved dependency blockers,
	// using `bd ready --json`.
	//
	// Preconditions:
	//   - ctx must be non-nil.
	//   - query.Limit, if > 0, caps results (same offset-window behavior as ListIssues).
	//
	// Postconditions:
	//   - Each returned issue has no unresolved dependency blockers at call time.
	//   - "Ready" is defined by bd as: status=open with no dependency blockers.
	//     Only issues with stored status "open" are eligible; bd ready explicitly
	//     excludes in_progress, blocked, deferred, hooked, and closed issues
	//     regardless of their dependency state (per bd 1.0.4 `bd ready --help`).
	//   - Results are not explicitly sorted by bd ready; callers should not
	//     assume any particular order.
	//
	// Error semantics:
	//   - ErrorCodeCommandFailed: bd exited non-zero.
	//   - ErrorCodeDecodeFailed: bd stdout was not parseable as JSON issue array.
	ReadyIssues(ctx context.Context, query domain.ReadyIssuesQuery) ([]domain.IssueSummary, error)

	// Query returns issue summaries using `bd query "<expr>" --json` with the bd query DSL.
	//
	// Preconditions:
	//   - ctx must be non-nil.
	//   - expr must be a non-empty, non-whitespace string. An empty or blank expr
	//     returns ErrorCodeValidationFailed without calling bd.
	//   - opts.IncludeClosed adds the -a flag to bd query, but for "status=closed"
	//     expressions bd already returns closed issues without -a. The flag is
	//     technically redundant for status=closed but harmless.
	//
	// Postconditions:
	//   - bd query supports the bd query DSL (e.g., "status=open", "status=in_progress",
	//     "status=closed"). Invalid DSL expressions cause bd to exit non-zero.
	//   - Sort direction translation (see ListIssues quirks): bd query --sort defaults
	//     to descending; the gateway emits --reverse only for SortDirectionAscending.
	//   - The board model uses Query("status=closed", {IncludeClosed:true, SortBy:
	//     SortFieldClosedAt, SortOrder:SortDirectionDescending, Limit:N}) to load the
	//     Done column. Result: newest-closed-first (bd's default DESC), as intended.
	//
	// Error semantics:
	//   - ErrorCodeValidationFailed: empty expression before any bd call.
	//   - ErrorCodeCommandFailed: bd exited non-zero (invalid DSL, bad expression).
	//   - ErrorCodeDecodeFailed: bd stdout was not parseable as JSON issue array.
	//
	// bd quirks observed (audit 2026-05-16):
	//   - bd query 'status=closed' --json returns closed issues WITHOUT the -a flag;
	//     -a is redundant when the expression explicitly selects closed. Disposition: ACCEPT.
	//   - Same sort-direction translation as bd list. Disposition: ACCEPT (zhef fix).
	Query(ctx context.Context, expr string, opts domain.QueryOptions) ([]domain.IssueSummary, error)

	// BlockedIssues returns issues that have at least one unresolved dependency blocker,
	// using `bd blocked --json`.
	//
	// Preconditions:
	//   - ctx must be non-nil.
	//   - query.Limit, if > 0, caps results via in-memory pagination after decoding.
	//
	// Postconditions:
	//   - Each returned BlockedIssueView.Issue has a non-empty ID, Title, Status, Type.
	//   - BlockedIssueView.BlockedBy is a slice of IssueReference with at least one
	//     element. Each reference has a non-empty ID (bare ID only; Title is empty
	//     because bd blocked --json returns blocked_by as a []string of bare IDs,
	//     not enriched objects).
	//   - IMPORTANT: "blocked" in this context means dependency-blocked (has unresolved
	//     dependency blockers). It is NOT the same as stored status "blocked". Issues
	//     returned here typically have stored status "open", not "blocked".
	//   - Issues with stored status "blocked" (manually set) are NOT returned by
	//     bd blocked unless they also have unresolved dependency blockers.
	//   - Use ListIssues(Statuses:["blocked"]) to fetch issues with stored status "blocked".
	//
	// Error semantics:
	//   - ErrorCodeCommandFailed: bd exited non-zero.
	//   - ErrorCodeDecodeFailed: bd stdout was not parseable as JSON issue array.
	//
	// bd quirks observed (audit 2026-05-16):
	//   - bd blocked --json returns blocked_by as []string (bare IDs), not []object.
	//     The decoder reads them as []string into bdIssuePayload.BlockedBy, and the
	//     gateway constructs IssueReference{ID: id} with only ID populated (Title,
	//     Priority, Status are zero). ReadyExplain's BlockedBy is richer (objects).
	//     Disposition: ACCEPT — consumers use only the ID for display/lookup.
	BlockedIssues(ctx context.Context, query domain.BlockedIssuesQuery) ([]domain.BlockedIssueView, error)

	// ReadyExplain returns both ready and dependency-blocked issues with aggregate
	// summary counts, using `bd ready --explain --json` in a single bd invocation.
	//
	// Preconditions:
	//   - ctx must be non-nil.
	//   - opts.Limit, if > 0, is forwarded as --limit to bd ready --explain.
	//     However, bd 1.0.4 ignores --limit in --explain mode and returns ALL ready
	//     and blocked issues regardless of the value passed. The flag is forwarded
	//     for argv cardinality tests and forward-compatibility, but has no observable
	//     effect on bd output in explain mode (confirmed in real-bd audit).
	//
	// Postconditions:
	//   - result.Ready contains issues with no unresolved dependency blockers.
	//   - result.Blocked contains issues with at least one unresolved dependency
	//     blocker; each entry has BlockedBy populated as enriched IssueReference
	//     objects (ID, Title, Priority, Status populated — richer than BlockedIssues).
	//   - result.TotalReady equals len(result.Ready) when no limit is applied.
	//     When opts.Limit > 0, TotalReady still equals len(result.Ready) (bd does
	//     not emit a separate total for the pre-limit population in this call).
	//   - result.TotalBlocked is the number of dependency-blocked issues.
	//   - result.CycleCount is the number of dependency cycles detected (typically 0).
	//   - result.Blocked[i].Issue.Status is typically "open" (not "blocked"), because
	//     dependency-blocked issues are not automatically given stored status "blocked".
	//
	// Error semantics:
	//   - ErrorCodeCommandFailed: bd exited non-zero.
	//   - ErrorCodeDecodeFailed: bd stdout was not parseable as the explain JSON object.
	//
	// bd quirks observed (audit 2026-05-16):
	//   - bd ready --explain --json returns a top-level JSON object (not an array),
	//     while bd ready --json returns a JSON array. These are decoded by different
	//     decoder paths. Disposition: ACCEPT — each uses the appropriate decoder.
	//   - schema_version field appears in the explain payload; decoder ignores it.
	//     Disposition: ACCEPT.
	//
	// bd quirks observed at scale (faif.5 audit 2026-05-17, ~590-issue corpus):
	//   - bd ready --explain --json ignores the default 100-item cap that
	//     bd ready --json applies. At ~590 issues with 507 ready, explain returns
	//     all 507 while plain bd ready --json returns only the first 100.
	//     ReadyExplain is the authoritative path for the board model (opts.Limit=0
	//     is always passed by the board). Disposition: ACCEPT — no gateway change
	//     needed; parity tests must use --limit 0 when calling BdReady directly.
	ReadyExplain(ctx context.Context, opts domain.ReadyExplainOptions) (domain.ReadyExplainResult, error)

	// ShowIssue returns full detail for a single issue using `bd show <id> --json`.
	//
	// Preconditions:
	//   - ctx must be non-nil.
	//   - query.IssueID must be a non-empty, non-whitespace string. An empty IssueID
	//     returns ErrorCodeValidationFailed without calling bd.
	//
	// Postconditions:
	//   - result.Summary has a non-empty ID, Title, Status, Type.
	//   - result.Description may be empty string ("") when the issue has no description
	//     set (see bd quirk below for the current error behavior).
	//   - result.ClosedAt is the zero time.Time when the issue is not closed.
	//   - result.CloseReason is empty string when the issue was not closed with a reason.
	//   - result.BlockedBy is populated from Dependencies with dependency_type "blocks"
	//     (or absent/other types that are not "related", "relates-to", or "parent-child").
	//   - result.Blocks is populated from Dependents with dependency_type "blocks".
	//   - result.Related is merged from "related"/"relates-to" dependency types in both
	//     Dependencies and Dependents arrays. The top-level Related array (bdIssuePayload.Related)
	//     is also merged in as a defensive measure: bd 1.0.4 does NOT emit a top-level
	//     "related" key in bd show --json output (all related relationships appear inside
	//     the dependencies/dependents arrays). The field is retained in case bd's schema
	//     evolves to include it in a future release; in current bd 1.0.4 it is always empty.
	//   - result.ParentGroupBrowser is populated when a parent-child dependency is
	//     present; the gateway issues a second bd show call for the parent to fetch
	//     siblings (cached per gateway instance across ShowIssue calls).
	//   - result.Comments is populated when the issue has comments.
	//
	// Error semantics:
	//   - ErrorCodeValidationFailed: empty IssueID before any bd call.
	//   - ErrorCodeCommandFailed: bd exited non-zero (unknown ID, bd bug, etc.).
	//     NOTE: bd show returns exit code 1 for unknown IDs (not a 404-equivalent
	//     exit code), so the gateway surfaces ErrorCodeCommandFailed, not
	//     ErrorCodeNotFound, for missing issues.
	//   - ErrorCodeDecodeFailed: bd stdout was not parseable, or a required field
	//     (id, title, status, issue_type, priority, created_at, updated_at) was absent.
	//
	// bd quirks observed (audit 2026-05-16):
	//   - bd show omits the "description" key entirely (not null, not "") when an issue
	//     has no description set. Disposition: ACCEPT (fixed in 781a 2026-05-16). The
	//     decoder now uses optionalString(primary.Description) → IssueDetail.Description
	//     becomes "" when bd omits the key. A pre-fix gateway version used requiredString
	//     and returned ErrorCodeDecodeFailed for any issue created without a description.
	//   - bd show returns exit code 1 for unknown IDs; there is no ErrorCodeNotFound
	//     path in the real gateway (only in the defensive zero-items branch).
	//     Disposition: ACCEPT — contract test already encodes this.
	//   - bd show --json wraps the single issue in a JSON array (not a bare object).
	//     The gateway reads items[0]. Disposition: ACCEPT.
	ShowIssue(ctx context.Context, query domain.ShowIssueQuery) (domain.IssueDetail, error)

	// SearchIssues returns issue search results. The routing strategy depends on query:
	//   - Empty text + no WorkState filter: routes through bd list --all --json.
	//   - WorkStateReady: routes through bd ready --json + in-memory filter.
	//   - WorkStateBlocked: routes through bd blocked --json + in-memory filter.
	//   - Non-empty text (or text + WorkState=Any): routes through bd search --json.
	//
	// Preconditions:
	//   - ctx must be non-nil.
	//   - If PriorityMin and PriorityMax are both set, PriorityMin <= PriorityMax;
	//     otherwise ErrorCodeValidationFailed is returned.
	//
	// Postconditions:
	//   - result.Results is never nil; it may be empty.
	//   - result.Metadata.Source indicates which bd command was used.
	//   - result.Metadata.Completeness is MaybeMore when bd search is used with a
	//     limit (bd search has a default cap of 50 unless --limit is passed); it is
	//     Partial when the returned count is less than the requested limit or no limit
	//     was requested.
	//   - When WorkState is Ready or Blocked and no text filter is set,
	//     result.Metadata.Notice is set to the no-text-filter notice string.
	//
	// Error semantics:
	//   - ErrorCodeValidationFailed: PriorityMin > PriorityMax.
	//   - ErrorCodeCommandFailed: underlying bd command exited non-zero.
	//   - ErrorCodeDecodeFailed: bd stdout was not parseable.
	//
	// bd quirks observed (audit 2026-05-16):
	//   - bd search requires a non-empty query argument; exits non-zero with
	//     {"error":"search query is required"} when no text is given. The gateway
	//     routes around this by using bd list --all for empty-text queries.
	//     Disposition: CARVE OUT — handled by routing logic; callers see normal results.
	//   - bd search --status accepts a SINGLE token only ("open", "in_progress", "all",
	//     etc.). Comma-separated values (e.g., "open,closed") return an empty array
	//     without an error. By contrast, bd list --status accepts comma-separated values.
	//     The gateway passes --status all when Statuses is empty and routes through
	//     bd search; when Statuses has multiple values and text is provided, the
	//     comma-joined string will silently return empty results.
	//     Disposition: INERT (closed as not-a-bug in 4fgn 2026-05-16). No UI path
	//     currently exposes multi-status + text filtering, so the silent-empty case
	//     is unreachable. Re-open if multi-status search is added to the UI.
	//   - bd search --json without --limit defaults to a cap of 50 results; this cap
	//     is a backend default, not a gateway-applied limit. Disposition: ACCEPT —
	//     metadata.Completeness reflects this via MaybeMore.
	SearchIssues(ctx context.Context, query domain.SearchIssuesQuery) (domain.SearchResultPage, error)

	// CountIssues returns issue counts by status using `bd count --by-status --json`.
	//
	// Preconditions:
	//   - ctx must be non-nil.
	//
	// Postconditions:
	//   - result.Total is the sum of all group counts in result.Groups.
	//   - result.Groups contains only status groups with a non-zero count (bd count
	//     omits zero-count groups from output).
	//   - All statuses (including closed) are included in the count WITHOUT requiring
	//     any --all flag; bd count --by-status is all-inclusive by default.
	//   - When query.Statuses is set, only the matching statuses are counted; result
	//     contains only groups for those statuses.
	//   - The IssueCountResult.Groups[i].Status values are the raw bd status names
	//     (e.g., "open", "in_progress", "blocked", "closed").
	//
	// Error semantics:
	//   - ErrorCodeCommandFailed: bd exited non-zero.
	//   - ErrorCodeDecodeFailed: bd stdout was not parseable as the count JSON object.
	//
	// bd quirks observed (audit 2026-05-16):
	//   - bd count --by-status --json includes all statuses by default (no --all needed),
	//     unlike bd list which requires --all or --status closed to see closed issues.
	//     Disposition: ACCEPT — documented as postcondition above.
	CountIssues(ctx context.Context, query domain.IssueCountQuery) (domain.IssueCountResult, error)

	// CreateIssue creates a new issue via `bd create --json`.
	//
	// Preconditions:
	//   - ctx must be non-nil.
	//   - input.Title must be non-empty; an empty title is passed to bd which exits 1
	//     with {"error":"title required..."} — the gateway does NOT pre-validate Title.
	//     The app layer is responsible for ensuring a non-empty title before calling.
	//   - input.Type must be a valid bd type name (e.g. "task", "bug", "feature") or empty;
	//     an invalid type causes bd to exit non-zero.
	//   - input.Labels elements must not contain commas; the gateway joins them with
	//     commas into a single --labels "a,b,c" flag and bd splits on commas.
	//
	// Postconditions:
	//   - On success, result.IssueID is the newly assigned issue ID (non-empty, trimmed).
	//   - The new issue is immediately visible to ShowIssue and ListIssues in subsequent
	//     calls (embedded dolt auto-commit ensures read-after-write consistency).
	//   - The issue is created with status "open" and the default priority (2) when
	//     input.Priority is nil.
	//   - When input.Labels is non-empty, labels are stored but NOT returned in the
	//     create response JSON; use ShowIssue to confirm labels were applied.
	//   - The created_by field is set to the git user.name of the repository; the gateway
	//     never passes --actor, so the caller cannot override actor attribution.
	//
	// Side effects:
	//   - A new issue record is written and committed to the dolt backend.
	//   - The new issue becomes visible to ListIssues, ReadyIssues, and CountIssues.
	//
	// Idempotency:
	//   - NOT idempotent. Each successful call creates a distinct issue with a new ID.
	//     Identical inputs produce different IDs because the timestamp differs.
	//
	// Error semantics:
	//   - ErrorCodeCommandFailed: bd exited non-zero (e.g. empty title, invalid type).
	//   - ErrorCodeDecodeFailed: bd stdout was not parseable as JSON, or the returned
	//     "id" field is empty.
	//
	// bd quirks observed (audit 2026-05-17):
	//   - bd create --json returns a full issue object; only the "id" field is used.
	//     Disposition: ACCEPT — createIssuePayload captures only "id".
	//   - Labels passed via --labels are NOT reflected in the create response JSON.
	//     Disposition: ACCEPT — callers needing label confirmation use ShowIssue.
	//   - bd create --json exits 1 and emits {"error":"title required..."} as JSON
	//     when --title is empty or omitted. Validation is the app layer's responsibility.
	//     Disposition: ACCEPT.
	CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error)

	// UpdateIssue updates an existing issue via `bd update <id>`.
	//
	// Preconditions:
	//   - ctx must be non-nil.
	//   - issueID must be non-empty, non-whitespace; an empty ID returns
	//     ErrorCodeValidationFailed without calling bd (validated by the gateway).
	//   - All pointer fields in input are optional; nil fields are omitted from the call.
	//     A pointer-to-empty-string (e.g. Description: ptr("")) clears that field in bd
	//     (confirmed: bd omits the key on next show after --description "").
	//   - input.Status, when non-nil, must be a valid bd status name; an invalid status
	//     causes bd to exit 1. The app layer pre-validates Status against StatusCatalog.
	//
	// Postconditions:
	//   - On success (nil error), updated fields are immediately visible to ShowIssue
	//     (embedded dolt auto-commit, read-after-write consistent).
	//   - A status transition to "in_progress" causes bd to set the started_at field
	//     on the issue; this is then visible in ShowIssue.
	//   - When all pointer fields are nil and ClearLabels is false, bd receives
	//     "bd update <id>" with no flags, exits 0 with "No updates specified" on stdout
	//     (not stderr), and the gateway returns nil error (silent no-op).
	//
	// Side effects:
	//   - updated_at advances after any non-no-op update.
	//   - Status transitions to "in_progress" also set started_at.
	//
	// Idempotency:
	//   - Applying the same update twice is safe; final state reflects the last call.
	//
	// Error semantics:
	//   - ErrorCodeValidationFailed: empty issueID before any bd call.
	//   - ErrorCodeCommandFailed: bd exited non-zero (unknown ID, invalid status/type, etc.).
	//
	// bd quirks observed (audit 2026-05-17):
	//   - bd update --set-labels "" silently does NOT clear labels (bd 1.0.4 bug).
	//     Disposition: WORKAROUND LANDED (see [[ubav]], fixed in py38). ClearLabels causes
	//     UpdateIssue to first fetch the issue's current labels via ShowIssue, then emit
	//     `bd update --remove-label <csv>` (singular flag, per bd 1.0.4's actual flag name)
	//     enumerating each label. If the issue has no labels, the bd update call is skipped
	//     entirely (nothing to remove).
	//   - bd update supports --json (returns updated issue as a JSON array). The gateway
	//     does NOT use --json; stdout is discarded. Disposition: ACCEPT.
	//   - "No updates specified" exits 0 on stdout (not stderr). Disposition: ACCEPT.
	//   - Unknown issueID: bd exits 1 with "Error resolving <id>: no issue found..."
	//     on stderr → ErrorCodeCommandFailed. No ErrorCodeNotFound path for writes.
	//     Disposition: ACCEPT.
	//   - Clearing description with --description "" works correctly (bd omits the key
	//     on next show → IssueDetail.Description=""). Disposition: ACCEPT.
	UpdateIssue(ctx context.Context, issueID string, input domain.UpdateIssueInput) error

	// CloseIssue closes an issue via `bd close <id>`.
	//
	// Preconditions:
	//   - ctx must be non-nil.
	//   - issueID must be non-empty, non-whitespace; an empty ID returns
	//     ErrorCodeValidationFailed without calling bd (validated by the gateway).
	//   - input.Reason is optional; when non-empty it is passed as --reason.
	//
	// Postconditions:
	//   - On success (nil error), the issue status is "closed" and the closed_at
	//     timestamp is set. Both are immediately visible to ShowIssue.
	//   - When input.Reason is non-empty, close_reason is stored on the issue
	//     and visible in ShowIssue. When empty, bd defaults close_reason to "Closed".
	//   - The closed issue no longer appears in ListIssues (default open filter)
	//     or ReadyIssues, but is returned by Query("status=closed", ...) and
	//     ListIssues with Statuses:["closed"] or Statuses:["all"].
	//
	// Side effects:
	//   - Issue status transitions to "closed"; closed_at and close_reason are set.
	//   - The issue is removed from the open/in_progress/blocked set for future reads.
	//
	// Idempotency:
	//   - IDEMPOTENT. Closing an already-closed issue exits 0 with the same success
	//     message. No error is returned for repeated close calls.
	//
	// Confirmation prompts:
	//   - bd close may prompt for confirmation when run interactively. The gateway
	//     always passes BD_NON_INTERACTIVE=1 via resolveEnv, which bypasses all
	//     interactive prompts. No prompt is expected during gateway use.
	//
	// Error semantics:
	//   - ErrorCodeValidationFailed: empty issueID before any bd call.
	//   - ErrorCodeCommandFailed: bd exited non-zero (unknown ID, etc.).
	//
	// bd quirks observed (audit 2026-05-17):
	//   - bd close is idempotent (exit 0 on re-close). Disposition: ACCEPT.
	//   - bd close supports --json (returns the closed issue as a JSON array). The gateway
	//     does NOT use --json; stdout is discarded. Disposition: ACCEPT.
	//   - When no --reason is supplied, bd stores close_reason as "Closed" (the literal
	//     string). This is bd's default, not an error. Disposition: ACCEPT.
	//   - Unknown issueID: bd exits 1 with "Error: resolving ID <id>: no issue found..."
	//     on stderr → ErrorCodeCommandFailed. No dedicated ErrorCodeNotFound for writes.
	//     Disposition: ACCEPT.
	CloseIssue(ctx context.Context, issueID string, input domain.CloseIssueInput) error

	// AddComment adds a comment to an issue via `bd comments add <id> <body>`.
	//
	// Preconditions:
	//   - ctx must be non-nil.
	//   - issueID must be non-empty, non-whitespace; an empty ID returns
	//     ErrorCodeValidationFailed without calling bd (validated by the gateway).
	//   - input.Body must be non-empty; the gateway does NOT pre-validate Body.
	//     An empty body causes bd to exit 1 with "comment text cannot be empty" on stderr.
	//     The app layer pre-validates Body before calling (see model.go mutationComment).
	//
	// Postconditions:
	//   - On success (nil error), the comment is immediately visible in ShowIssue
	//     (embedded dolt auto-commit, read-after-write consistent).
	//   - The comment's author is set to the git user.name of the repository; the
	//     gateway never passes --actor.
	//   - Comments can be added to closed issues; bd does not guard against this.
	//   - Very long bodies (tested at 1000 chars) are accepted without error.
	//   - Markdown content (headings, lists, bold, code, blockquotes) is accepted
	//     verbatim and stored as-is; no escaping or stripping occurs.
	//
	// Side effects:
	//   - A new comment record is appended to the issue's comment list.
	//   - The issue's updated_at is NOT advanced by a comment addition (bd comment
	//     is stored separately from the issue core record).
	//
	// Idempotency:
	//   - NOT idempotent. Each call appends a new comment entry; duplicate calls
	//     produce duplicate comments.
	//
	// Error semantics:
	//   - ErrorCodeValidationFailed: empty issueID before any bd call.
	//   - ErrorCodeCommandFailed: bd exited non-zero (unknown ID, empty body, etc.).
	//
	// bd quirks observed (audit 2026-05-17):
	//   - bd comments add does not support --json; stdout is always "Comment added to <id>"
	//     (human-readable). The gateway discards stdout. Disposition: ACCEPT.
	//   - Empty body exits 1 with "Error: comment text cannot be empty" on stderr.
	//     The gateway surfaces ErrorCodeCommandFailed; the app layer prevents this via
	//     pre-validation. Disposition: ACCEPT.
	//   - Comments on closed issues succeed (exit 0); bd does not enforce open-only
	//     commenting. Disposition: ACCEPT.
	//   - bd comments add supports --actor "<name>" to override the author field. The
	//     gateway never passes --actor; author attribution always uses git user.name.
	//     Disposition: ACCEPT.
	AddComment(ctx context.Context, issueID string, input domain.AddCommentInput) error

	// StatusCatalog returns all available issue statuses using `bd statuses --json`.
	//
	// Preconditions:
	//   - ctx must be non-nil.
	//
	// Postconditions:
	//   - The result is never empty; bd always returns at least the built-in statuses.
	//   - Built-in statuses always include at least: open, in_progress, blocked,
	//     deferred, closed, pinned, hooked.
	//   - Custom project-level statuses appear after built-in statuses in the result.
	//   - Each StatusOption.Name is non-empty.
	//   - StatusOption.Description is the human-readable description (may be empty for
	//     custom statuses that have no description).
	//
	// Error semantics:
	//   - ErrorCodeCommandFailed: bd exited non-zero.
	//   - ErrorCodeDecodeFailed: bd stdout was not parseable as the statuses JSON object.
	//
	// bd quirks observed (audit 2026-05-16):
	//   - bd statuses --json omits the "custom_statuses" key entirely when no custom
	//     statuses are configured (rather than returning an empty array). The decoder
	//     handles this correctly (nil slice). Disposition: ACCEPT.
	//   - bd statuses --json includes "category" and "icon" fields not captured by the
	//     decoder. Disposition: ACCEPT — not needed by current consumers.
	StatusCatalog(ctx context.Context) ([]domain.StatusOption, error)

	// TypeCatalog returns available issue types using `bd types --json`.
	//
	// Preconditions:
	//   - ctx must be non-nil.
	//
	// Postconditions:
	//   - The result is never empty; bd always returns at least the core types.
	//   - Core types always include at least: task, bug, feature, chore, epic,
	//     decision, spike, story, milestone.
	//   - Custom types appear after core types in the result.
	//   - Each TypeOption.Name is non-empty.
	//
	// Error semantics:
	//   - ErrorCodeCommandFailed: bd exited non-zero.
	//   - ErrorCodeDecodeFailed: bd stdout was not parseable as the types JSON object.
	//
	// bd quirks observed (audit 2026-05-16):
	//   - bd types --json omits the "custom_types" key entirely when no custom types
	//     are configured. The decoder handles this correctly (nil slice). Disposition: ACCEPT.
	//   - When custom types ARE configured (via `bd config set types.custom <value>`),
	//     bd 1.0.4 returns "custom_types" as a JSON array of bare strings
	//     (e.g. ["widget","gadget"]), not objects with name/description fields.
	//     The decoder accepts both shapes as of puy3 (parent epic yspw): bare strings
	//     are decoded as TypeOption{Name: s} with no description. Disposition: ACCEPT.
	TypeCatalog(ctx context.Context) ([]domain.TypeOption, error)

	// LabelCatalog returns all labels in use across the database using
	// `bd label list-all --json`.
	//
	// Preconditions:
	//   - ctx must be non-nil.
	//
	// Postconditions:
	//   - Each LabelOption.Name is non-empty (the gateway strips whitespace and
	//     skips blank entries from the bd output).
	//   - Labels are returned in the order bd provides (typically alphabetical or
	//     by usage count — unspecified).
	//   - Returns an empty slice (not an error) when no labels exist in the database.
	//
	// Error semantics:
	//   - ErrorCodeCommandFailed: bd exited non-zero.
	//   - ErrorCodeDecodeFailed: bd stdout was not parseable as a JSON array of
	//     label entries.
	//
	// bd quirks observed (audit 2026-05-16):
	//   - bd label list-all --json returns a JSON array where each entry has "label"
	//     (not "name") and "count" fields. The decoder uses bdLabelCatalogEntryPayload
	//     with json:"label" — this is intentionally different from the catalog entry
	//     pattern used by statuses/types (which use "name"). Disposition: ACCEPT.
	//   - Labels from closed issues are included in the label catalog.
	//     Disposition: ACCEPT.
	LabelCatalog(ctx context.Context) ([]domain.LabelOption, error)
}
