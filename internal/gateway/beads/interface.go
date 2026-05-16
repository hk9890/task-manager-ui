// Package beads defines the BeadsGateway interface and its CLI-backed implementation.
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
package beads

import (
	"context"

	"github.com/hk9890/beads-workbench/internal/domain"
)

// BeadsGateway is the source-specific beads gateway used by the Beads Workbench UI.
// A gateway instance is bound to one beads source/project.
//
// All read methods on BeadsGateway are safe for concurrent use.
// Write methods (CreateIssue, UpdateIssue, CloseIssue, AddComment) are not
// included in this contract; their spec is tracked in epic beads-workbench-9x70.
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
	//   - "Ready" is defined by the dependency graph, not the stored status field.
	//     An issue with stored status "open" and no blocking deps is ready.
	//     An issue with stored status "in_progress" and no blocking deps is also
	//     returned by bd ready (bd ready is not limited to status=open).
	//   - Closed issues are excluded regardless of their dependency state.
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
	//     The limit applies to the ready list; its effect on the blocked list is
	//     unspecified by bd.
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
	//     Dependencies and Dependents arrays, plus the top-level Related array.
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

	// CreateIssue creates a new issue via bd create.
	// Write-side contract pending; see epic beads-workbench-9x70.
	CreateIssue(ctx context.Context, input domain.CreateIssueInput) (domain.CreateIssueResult, error)

	// UpdateIssue updates an existing issue via bd update.
	// Write-side contract pending; see epic beads-workbench-9x70.
	UpdateIssue(ctx context.Context, issueID string, input domain.UpdateIssueInput) error

	// CloseIssue closes an issue via bd close.
	// Write-side contract pending; see epic beads-workbench-9x70.
	CloseIssue(ctx context.Context, issueID string, input domain.CloseIssueInput) error

	// AddComment adds a comment to an issue via bd comments add.
	// Write-side contract pending; see epic beads-workbench-9x70.
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
