// Package main implements bwb-smoke, a one-command release smoke binary that
// compares what bwb's data layer reports against bd CLI output on the same database.
//
// Usage:
//
//	bwb-smoke --dir <path> [--readonly] [--format text|json] [--checks count,sort,search,render]
//
// Exit codes: 0 if all selected checks PASS; 1 if any check FAILS or an error occurs.
//
// Safety: --readonly=true (the default) prepends --readonly to every bd invocation
// and sets ReadOnly:true on the repository runner, preventing any writes to the target DB.
// Always use --readonly when pointing at a shared or production database.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	beads "github.com/hk9890/beads-workbench/internal/bd"
	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/dashboard"
	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/mode/board"
	"github.com/hk9890/beads-workbench/internal/repository"
	repobeads "github.com/hk9890/beads-workbench/internal/repository/beads"
	memoryrepo "github.com/hk9890/beads-workbench/internal/repository/memory"
)

// allChecks is the canonical ordered list of check names.
var allChecks = []string{"count", "sort", "search", "render"}

// closedCapForSmoke mirrors the minimum cap enforced by board.Model.closedLimit()
// (max(50, sectionItemCapacity)); 50 is the guaranteed floor.
const closedCapForSmoke = 50

// paritySearchLimit is the page size used for search parity queries.
const paritySearchLimit = 20

// firstNIDs is the number of leading IDs compared in the search check.
const firstNIDs = 10

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	var (
		dir      string
		readonly bool
		format   string
		checks   string
	)

	fs := flag.NewFlagSet("bwb-smoke", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&dir, "dir", "", "DB working directory (defaults to cwd). Safe to omit for this-repo checks.")
	fs.BoolVar(&readonly, "readonly", true, "Never write to the target DB (prepends --readonly to all bd calls). Default true — always use true against production or shared DBs.")
	fs.StringVar(&format, "format", "text", "Output format: text or json")
	fs.StringVar(&checks, "checks", strings.Join(allChecks, ","), "Comma-separated list of checks to run: count,sort,search,render")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "unexpected arguments: %v\n", fs.Args())
		fs.Usage()
		return 2
	}

	// Resolve --dir
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "failed to resolve cwd: %v\n", err)
			return 1
		}
		dir = cwd
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "failed to resolve --dir %q: %v\n", dir, err)
		return 1
	}
	if info, err := os.Stat(absDir); err != nil || !info.IsDir() {
		_, _ = fmt.Fprintf(stderr, "--dir %q is not accessible: %v\n", absDir, err)
		return 1
	}

	// Validate --format
	if format != "text" && format != "json" {
		_, _ = fmt.Fprintf(stderr, "--format must be text or json, got %q\n", format)
		return 2
	}

	// Parse --checks
	selectedChecks, err := parseChecks(checks)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "--checks: %v\n", err)
		return 2
	}

	// Build repository
	runner := beads.NewCommandRunner(beads.RunnerConfig{
		WorkDir:  absDir,
		ReadOnly: readonly,
	})
	repo := repobeads.New(runner)

	// Run selected checks
	var results []CheckResult
	for _, name := range allChecks {
		if !containsStr(selectedChecks, name) {
			continue
		}
		var r CheckResult
		switch name {
		case "count":
			r = runCountCheck(absDir, repo, readonly)
		case "sort":
			r = runSortCheck(absDir, repo, readonly)
		case "search":
			r = runSearchCheck(absDir, repo, readonly)
		case "render":
			r = runRenderCheck()
		}
		results = append(results, r)
	}

	// Determine overall result
	overallPass := true
	for _, r := range results {
		if r.Status != "PASS" {
			overallPass = false
			break
		}
	}
	overallStr := "PASS"
	if !overallPass {
		overallStr = "FAIL"
	}

	// Emit report
	switch format {
	case "json":
		emitJSON(stdout, absDir, results, overallStr)
	default:
		emitText(stdout, absDir, results, overallStr)
	}

	if overallPass {
		return 0
	}
	return 1
}

// CheckResult holds the outcome of a single smoke check.
type CheckResult struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "PASS" or "FAIL"
	Detail string `json:"detail"`
}

// parseChecks validates and deduplicates the comma-separated check list.
func parseChecks(raw string) ([]string, error) {
	valid := make(map[string]struct{})
	for _, c := range allChecks {
		valid[c] = struct{}{}
	}
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{})
	var out []string
	for _, p := range parts {
		name := strings.TrimSpace(p)
		if name == "" {
			continue
		}
		if _, ok := valid[name]; !ok {
			return nil, fmt.Errorf("unknown check %q (valid: %s)", name, strings.Join(allChecks, ", "))
		}
		if _, already := seen[name]; already {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("at least one check must be selected")
	}
	return out, nil
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// ── count check ──────────────────────────────────────────────────────────────

func runCountCheck(dir string, repo repository.Repository, readonly bool) CheckResult {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// bwb side: call repo.Dashboard which fans out 5 bd calls in parallel.
	cols, err := runDashboardFetch(ctx, repo)
	if err != nil {
		return CheckResult{Name: "count", Status: "FAIL", Detail: fmt.Sprintf("repository fetch: %v", err)}
	}

	// bd side: count --by-status --json
	countRaw, err := bdRun(dir, readonly, "count", "--by-status", "--json")
	if err != nil {
		return CheckResult{Name: "count", Status: "FAIL", Detail: fmt.Sprintf("bd count: %v", err)}
	}
	statusCounts, err := parseBdCountByStatus(countRaw)
	if err != nil {
		return CheckResult{Name: "count", Status: "FAIL", Detail: fmt.Sprintf("parse bd count: %v", err)}
	}

	// Pass --limit 0 to match ReadyExplain's uncapped output: bd ready --json
	// without --limit 0 caps at 100 while bd ready --explain (used by the Repository)
	// returns all ready issues. See interface.go "bd quirks observed at scale".
	readyRaw, err := bdRun(dir, readonly, "ready", "--limit", "0", "--json")
	if err != nil {
		return CheckResult{Name: "count", Status: "FAIL", Detail: fmt.Sprintf("bd ready: %v", err)}
	}
	bdReadyCount, err := parseBdIssueArrayLen(readyRaw)
	if err != nil {
		return CheckResult{Name: "count", Status: "FAIL", Detail: fmt.Sprintf("parse bd ready: %v", err)}
	}

	// NotReady includes both dependency-blocked issues (from bd blocked — blocked
	// by unresolved dependencies, regardless of stored status) and stored-blocked
	// issues (status=blocked in bd with no dependency blocker). bwb deduplicates
	// the union. To match, the bd-side count is the deduplicated union of:
	//   - IDs from "bd blocked" (dep-blocked issues)
	//   - IDs from "bd list --status blocked" (status=blocked issues)
	blockedRaw, err := bdRun(dir, readonly, "blocked", "--json")
	if err != nil {
		return CheckResult{Name: "count", Status: "FAIL", Detail: fmt.Sprintf("bd blocked: %v", err)}
	}
	blockedItems, err := parseBdMinimalArray(blockedRaw)
	if err != nil {
		return CheckResult{Name: "count", Status: "FAIL", Detail: fmt.Sprintf("parse bd blocked: %v", err)}
	}

	storedBlockedRaw, err := bdRun(dir, readonly, "list", "--status", "blocked", "--json")
	if err != nil {
		return CheckResult{Name: "count", Status: "FAIL", Detail: fmt.Sprintf("bd list --status blocked: %v", err)}
	}
	storedBlockedItems, err := parseBdMinimalArray(storedBlockedRaw)
	if err != nil {
		return CheckResult{Name: "count", Status: "FAIL", Detail: fmt.Sprintf("parse bd list --status blocked: %v", err)}
	}

	// Deduplicate the union of dep-blocked and stored-blocked IDs.
	notReadyIDs := make(map[string]struct{}, len(blockedItems)+len(storedBlockedItems))
	for _, item := range blockedItems {
		notReadyIDs[item.ID] = struct{}{}
	}
	for _, item := range storedBlockedItems {
		notReadyIDs[item.ID] = struct{}{}
	}
	bdNotReadyCount := len(notReadyIDs)

	var mismatches []string

	if cols.NotReady.Total != bdNotReadyCount {
		mismatches = append(mismatches, fmt.Sprintf("NotReady: bwb=%d bd=%d (delta=%d)",
			cols.NotReady.Total, bdNotReadyCount, cols.NotReady.Total-bdNotReadyCount))
	}
	if cols.Ready.Total != bdReadyCount {
		mismatches = append(mismatches, fmt.Sprintf("Ready: bwb=%d bd=%d (delta=%d)",
			cols.Ready.Total, bdReadyCount, cols.Ready.Total-bdReadyCount))
	}
	bdInProgress := statusCounts["in_progress"]
	if cols.InProgress.Total != bdInProgress {
		mismatches = append(mismatches, fmt.Sprintf("InProgress: bwb=%d bd=%d (delta=%d)",
			cols.InProgress.Total, bdInProgress, cols.InProgress.Total-bdInProgress))
	}
	bdClosed := statusCounts["closed"]
	if cols.Done.Total != bdClosed {
		mismatches = append(mismatches, fmt.Sprintf("Done: bwb=%d bd=%d (delta=%d)",
			cols.Done.Total, bdClosed, cols.Done.Total-bdClosed))
	}

	if len(mismatches) == 0 {
		detail := fmt.Sprintf("NotReady=%d Ready=%d InProgress=%d Done=%d — all match",
			cols.NotReady.Total, cols.Ready.Total, cols.InProgress.Total, cols.Done.Total)
		return CheckResult{Name: "count", Status: "PASS", Detail: detail}
	}
	return CheckResult{Name: "count", Status: "FAIL", Detail: strings.Join(mismatches, "; ")}
}

// ── sort check ───────────────────────────────────────────────────────────────

func runSortCheck(dir string, repo repository.Repository, readonly bool) CheckResult {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cols, err := runDashboardFetch(ctx, repo)
	if err != nil {
		return CheckResult{Name: "sort", Status: "FAIL", Detail: fmt.Sprintf("repository fetch: %v", err)}
	}

	var mismatches []string

	// Ready: bd ready --json, apply issueSort to both sides.
	// Pass --limit 0 to match ReadyExplain's uncapped output (bd default caps at 100).
	// See interface.go "bd quirks observed at scale".
	readyRaw, err := bdRun(dir, readonly, "ready", "--limit", "0", "--json")
	if err != nil {
		return CheckResult{Name: "sort", Status: "FAIL", Detail: fmt.Sprintf("bd ready: %v", err)}
	}
	bdReadyItems, err := parseBdSortableArray(readyRaw)
	if err != nil {
		return CheckResult{Name: "sort", Status: "FAIL", Detail: fmt.Sprintf("parse bd ready: %v", err)}
	}
	applyIssueSort(bdReadyItems)
	if msg := compareIDOrder("Ready", issueIDs(cols.Ready.Issues), sortableIDs(bdReadyItems)); msg != "" {
		mismatches = append(mismatches, msg)
	}

	// NotReady: bd blocked --json, apply issueSort to bd side.
	blockedRaw, err := bdRun(dir, readonly, "blocked", "--json")
	if err != nil {
		return CheckResult{Name: "sort", Status: "FAIL", Detail: fmt.Sprintf("bd blocked: %v", err)}
	}
	bdBlockedItems, err := parseBdSortableArray(blockedRaw)
	if err != nil {
		return CheckResult{Name: "sort", Status: "FAIL", Detail: fmt.Sprintf("parse bd blocked: %v", err)}
	}
	applyIssueSort(bdBlockedItems)
	if msg := compareIDOrder("NotReady", issueIDs(cols.NotReady.Issues), sortableIDs(bdBlockedItems)); msg != "" {
		mismatches = append(mismatches, msg)
	}

	// InProgress: bd list --status in_progress --json, apply issueSort to bd side.
	inProgressRaw, err := bdRun(dir, readonly, "list", "--status", "in_progress", "--json")
	if err != nil {
		return CheckResult{Name: "sort", Status: "FAIL", Detail: fmt.Sprintf("bd list in_progress: %v", err)}
	}
	bdIPItems, err := parseBdSortableArray(inProgressRaw)
	if err != nil {
		return CheckResult{Name: "sort", Status: "FAIL", Detail: fmt.Sprintf("parse bd list in_progress: %v", err)}
	}
	applyIssueSort(bdIPItems)
	if msg := compareIDOrder("InProgress", issueIDs(cols.InProgress.Issues), sortableIDs(bdIPItems)); msg != "" {
		mismatches = append(mismatches, msg)
	}

	// Done: bd query 'status=closed' -a --sort closed --limit <cap> --json
	doneRaw, err := bdQueryClosed(dir, readonly, closedCapForSmoke)
	if err != nil {
		return CheckResult{Name: "sort", Status: "FAIL", Detail: fmt.Sprintf("bd query closed: %v", err)}
	}
	bdDoneItems, err := parseBdSortableArray(doneRaw)
	if err != nil {
		return CheckResult{Name: "sort", Status: "FAIL", Detail: fmt.Sprintf("parse bd query closed: %v", err)}
	}
	// Done column: no re-sort; compare position-by-position.
	if msg := compareIDOrder("Done", issueIDs(cols.Done.Issues), sortableIDs(bdDoneItems)); msg != "" {
		mismatches = append(mismatches, msg)
	}

	if len(mismatches) == 0 {
		return CheckResult{Name: "sort", Status: "PASS", Detail: "all 4 columns match bd order"}
	}
	return CheckResult{Name: "sort", Status: "FAIL", Detail: strings.Join(mismatches, "; ")}
}

// ── search check ─────────────────────────────────────────────────────────────

// searchQueryCase describes one entry in the smoke search matrix.
type searchQueryCase struct {
	name  string
	query domain.SearchIssuesQuery
}

var smokeSearchMatrix = []searchQueryCase{
	{name: "empty", query: domain.SearchIssuesQuery{Limit: paritySearchLimit}},
	{name: "text=test", query: domain.SearchIssuesQuery{Text: "test", Limit: paritySearchLimit}},
	{name: "text=fix", query: domain.SearchIssuesQuery{Text: "fix", Limit: paritySearchLimit}},
	{name: "text=render", query: domain.SearchIssuesQuery{Text: "render", Limit: paritySearchLimit}},
}

func runSearchCheck(dir string, repo repository.Repository, readonly bool) CheckResult {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var mismatches []string
	for _, qc := range smokeSearchMatrix {
		page, err := repo.Search(ctx, qc.query)
		if err != nil {
			mismatches = append(mismatches, fmt.Sprintf("[%s] repository: %v", qc.name, err))
			continue
		}

		// Compare against bd
		var bdRaw []byte
		var bdErr error
		if strings.TrimSpace(qc.query.Text) == "" {
			// Empty query: bd list --json --all [--limit N]
			args := []string{"--all"}
			if qc.query.Limit > 0 {
				args = append(args, "--limit", strconv.Itoa(qc.query.Limit))
			}
			bdRaw, bdErr = bdRun(dir, readonly, "list", args...)
		} else {
			// Text query: bd search <text> --json --status all [--limit N]
			args := []string{strings.TrimSpace(qc.query.Text), "--status", "all"}
			if qc.query.Limit > 0 {
				args = append(args, "--limit", strconv.Itoa(qc.query.Limit))
			}
			bdRaw, bdErr = bdRun(dir, readonly, "search", args...)
		}
		if bdErr != nil {
			mismatches = append(mismatches, fmt.Sprintf("[%s] bd: %v", qc.name, bdErr))
			continue
		}

		bdItems, err := parseBdMinimalArray(bdRaw)
		if err != nil {
			mismatches = append(mismatches, fmt.Sprintf("[%s] parse bd: %v", qc.name, err))
			continue
		}

		gwIDs := searchResultIDs(page.Results)
		bdIDs := minimalIDs(bdItems)

		// Count parity
		if len(gwIDs) != len(bdIDs) {
			mismatches = append(mismatches, fmt.Sprintf("[%s] count: gw=%d bd=%d", qc.name, len(gwIDs), len(bdIDs)))
			continue
		}

		// Leading ID parity
		n := firstNIDs
		if len(gwIDs) < n {
			n = len(gwIDs)
		}
		for i := 0; i < n; i++ {
			if gwIDs[i] != bdIDs[i] {
				mismatches = append(mismatches, fmt.Sprintf("[%s] order diverges at index %d: gw=%s bd=%s", qc.name, i, gwIDs[i], bdIDs[i]))
				break
			}
		}
	}

	if len(mismatches) == 0 {
		return CheckResult{Name: "search", Status: "PASS", Detail: fmt.Sprintf("%d queries all match", len(smokeSearchMatrix))}
	}
	return CheckResult{Name: "search", Status: "FAIL", Detail: strings.Join(mismatches, "; ")}
}

// ── render check ─────────────────────────────────────────────────────────────

func runRenderCheck() CheckResult {
	keys, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
	if err != nil {
		return CheckResult{Name: "render", Status: "FAIL", Detail: fmt.Sprintf("keybindings: %v", err)}
	}

	// Build a board model with an empty memory repository.
	// Data is fed directly via board.FeedTestData (bypasses async dispatch).
	m := board.NewModel(memoryrepo.New(), slog.Default(), keys)

	type step struct {
		label string
		want  int
	}

	var failures []string

	// Step 1: SetSize — loading columns, no data yet.
	m.SetSize(180, 30)
	got := countColumnTopBorders(m.View(0))
	if got != 4 {
		failures = append(failures, fmt.Sprintf("step1(SetSize 180x30): got %d ╭ want 4", got))
	}

	// Step 2: Resize (simulate resize before data).
	m.SetSize(200, 40)
	got = countColumnTopBorders(m.View(0))
	if got != 4 {
		failures = append(failures, fmt.Sprintf("step2(resize 200x40): got %d ╭ want 4", got))
	}

	// Step 3: Feed data at current size.
	feedRenderData(m)
	got = countColumnTopBorders(m.View(0))
	if got != 4 {
		failures = append(failures, fmt.Sprintf("step3(data at 200x40): got %d ╭ want 4", got))
	}

	// Step 4: Second resize.
	m.SetSize(180, 30)
	got = countColumnTopBorders(m.View(0))
	if got != 4 {
		failures = append(failures, fmt.Sprintf("step4(resize 180x30): got %d ╭ want 4", got))
	}

	if len(failures) == 0 {
		return CheckResult{Name: "render", Status: "PASS", Detail: "4 captures all show exactly 4 column-top borders (╭)"}
	}
	return CheckResult{Name: "render", Status: "FAIL", Detail: strings.Join(failures, "; ")}
}

// countColumnTopBorders counts occurrences of the box-drawing top-left corner
// character (╭) in the rendered view. A correct full-board render at a wide
// enough terminal has exactly 4 occurrences — one per column header.
func countColumnTopBorders(view string) int {
	return strings.Count(view, "╭")
}

// feedRenderData drives all four Repository result messages into the board model,
// simulating a completed board load. This mirrors feedAllColumnResults from the
// render_regression_test.go but operates via the exported board.Model.Update path.
//
// Because the render check uses a real Repository (not a recording fake), we
// call board.FeedTestData which is exported from the board package for this purpose.
//
// If board.FeedTestData is not available, we fall back to driving the model via
// repeated Update calls with the public tea.WindowSizeMsg to flush pending state.
func feedRenderData(m *board.Model) {
	board.FeedTestData(m)
}

// ── dashboard fetch (shared by count + sort) ─────────────────────────────────

func runDashboardFetch(ctx context.Context, repo repository.Repository) (dashboard.Columns, error) {
	data, err := repo.Dashboard(ctx)
	if err != nil {
		return dashboard.Columns{}, fmt.Errorf("dashboard: %w", err)
	}

	return dashboard.Compose(dashboard.Inputs{
		Ready:         data.ReadyExplain.Ready,
		Blocked:       data.ReadyExplain.Blocked,
		StoredBlocked: data.Blocked,
		InProgress:    data.InProgress,
		Closed:        data.Closed,
		ClosedLimit:   closedCapForSmoke,
		ClosedTotal:   data.ClosedTotal,
	}), nil
}

// ── bd helpers ───────────────────────────────────────────────────────────────

// bdRun executes a bd command from dir, optionally prepending --readonly,
// and appends --json. Returns stdout bytes.
func bdRun(dir string, readonly bool, verb string, extraArgs ...string) ([]byte, error) {
	argv := make([]string, 0, len(extraArgs)+3)
	if readonly {
		argv = append(argv, "--readonly")
	}
	argv = append(argv, verb)
	argv = append(argv, extraArgs...)
	argv = append(argv, "--json")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bd", argv...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
	return cmd.Output()
}

// bdQueryClosed mirrors the exact bd query call the Repository uses for the Done column.
func bdQueryClosed(dir string, readonly bool, limit int) ([]byte, error) {
	argv := make([]string, 0, 10)
	if readonly {
		argv = append(argv, "--readonly")
	}
	argv = append(argv,
		"query", "status=closed",
		"-a",
		"--sort", "closed",
		"--limit", strconv.Itoa(limit),
		"--json",
	)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bd", argv...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
	return cmd.Output()
}

// ── JSON decoders ─────────────────────────────────────────────────────────────

type bdCountByStatusResult struct {
	Groups []struct {
		Count int    `json:"count"`
		Group string `json:"group"`
	} `json:"groups"`
	Total int `json:"total"`
}

func parseBdCountByStatus(raw []byte) (map[string]int, error) {
	var result bdCountByStatusResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("unmarshal count: %w", err)
	}
	out := make(map[string]int, len(result.Groups))
	for _, g := range result.Groups {
		out[g.Group] = g.Count
	}
	return out, nil
}

type bdMinimalIssue struct {
	ID        string `json:"id"`
	Priority  int    `json:"priority"`
	UpdatedAt string `json:"updated_at"`
	Status    string `json:"status"`
	Title     string `json:"title"`
}

func parseBdIssueArrayLen(raw []byte) (int, error) {
	items, err := parseBdMinimalArray(raw)
	if err != nil {
		return 0, err
	}
	return len(items), nil
}

func parseBdMinimalArray(raw []byte) ([]bdMinimalIssue, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var items []bdMinimalIssue
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, fmt.Errorf("unmarshal array: %w", err)
		}
		return items, nil
	}
	// Wrapper object: try common keys
	var wrapper struct {
		Issues  []bdMinimalIssue `json:"issues"`
		Results []bdMinimalIssue `json:"results"`
		Items   []bdMinimalIssue `json:"items"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("unmarshal wrapper: %w", err)
	}
	if len(wrapper.Issues) > 0 {
		return wrapper.Issues, nil
	}
	if len(wrapper.Results) > 0 {
		return wrapper.Results, nil
	}
	return wrapper.Items, nil
}

type bdSortableIssue struct {
	ID        string `json:"id"`
	Priority  int    `json:"priority"`
	UpdatedAt string `json:"updated_at"`
}

func parseBdSortableArray(raw []byte) ([]bdSortableIssue, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var items []bdSortableIssue
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, fmt.Errorf("unmarshal sortable array: %w", err)
		}
		return items, nil
	}
	// Wrapper
	var wrapper struct {
		Issues  []bdSortableIssue `json:"issues"`
		Results []bdSortableIssue `json:"results"`
		Items   []bdSortableIssue `json:"items"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("unmarshal sortable wrapper: %w", err)
	}
	if len(wrapper.Issues) > 0 {
		return wrapper.Issues, nil
	}
	if len(wrapper.Results) > 0 {
		return wrapper.Results, nil
	}
	return wrapper.Items, nil
}

// applyIssueSort sorts items in-place using the same logic as dashboard.issueSort.
func applyIssueSort(items []bdSortableIssue) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		at, _ := time.Parse(time.RFC3339, a.UpdatedAt)
		bt, _ := time.Parse(time.RFC3339, b.UpdatedAt)
		if !at.Equal(bt) {
			return at.After(bt)
		}
		return a.ID < b.ID
	})
}

func sortableIDs(items []bdSortableIssue) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func issueIDs(issues []domain.IssueSummary) []string {
	ids := make([]string, 0, len(issues))
	for _, issue := range issues {
		ids = append(ids, issue.ID)
	}
	return ids
}

func minimalIDs(items []bdMinimalIssue) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func searchResultIDs(results []domain.SearchResult) []string {
	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.Issue.ID)
	}
	return ids
}

// compareIDOrder compares two ID slices up to min(len(a), len(b)) entries.
// Returns a non-empty failure message on the first divergence.
func compareIDOrder(column string, bwbIDs, bdIDs []string) string {
	n := len(bwbIDs)
	if len(bdIDs) < n {
		n = len(bdIDs)
	}
	if n == 0 {
		return ""
	}
	for i := 0; i < n; i++ {
		if bwbIDs[i] != bdIDs[i] {
			pre := 5
			if n < pre {
				pre = n
			}
			return fmt.Sprintf("%s: diverge at index %d bwb=%s bd=%s (bwb[0:%d]=%v bd[0:%d]=%v)",
				column, i, bwbIDs[i], bdIDs[i], pre, bwbIDs[:pre], pre, bdIDs[:pre])
		}
	}
	return ""
}

// ── output formatters ─────────────────────────────────────────────────────────

func emitText(w *os.File, dir string, results []CheckResult, overall string) {
	_, _ = fmt.Fprintf(w, "bwb-smoke report for %s\n\n", dir)
	_, _ = fmt.Fprintf(w, "%-10s  %-6s  %s\n", "check", "status", "detail")
	_, _ = fmt.Fprintf(w, "%-10s  %-6s  %s\n", "----------", "------", "------")
	for _, r := range results {
		_, _ = fmt.Fprintf(w, "%-10s  %-6s  %s\n", r.Name, r.Status, r.Detail)
	}
	_, _ = fmt.Fprintf(w, "\nresult: %s\n", overall)
}

type jsonReport struct {
	Dir    string        `json:"dir"`
	Checks []CheckResult `json:"checks"`
	Result string        `json:"result"`
}

func emitJSON(w *os.File, dir string, results []CheckResult, overall string) {
	report := jsonReport{
		Dir:    dir,
		Checks: results,
		Result: overall,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(report)
}
