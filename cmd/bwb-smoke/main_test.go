package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// ── flag parsing ──────────────────────────────────────────────────────────────

func TestParseFlagsDefaults(t *testing.T) {
	// parseChecks with the default all-checks string must return all 4.
	checks, err := parseChecks(strings.Join(allChecks, ","))
	if err != nil {
		t.Fatalf("parseChecks(default): %v", err)
	}
	if len(checks) != 4 {
		t.Errorf("expected 4 checks, got %d: %v", len(checks), checks)
	}
	for i, want := range allChecks {
		if checks[i] != want {
			t.Errorf("check[%d]: want %q got %q", i, want, checks[i])
		}
	}
}

func TestParseChecksSubset(t *testing.T) {
	checks, err := parseChecks("count,render")
	if err != nil {
		t.Fatalf("parseChecks subset: %v", err)
	}
	if len(checks) != 2 {
		t.Errorf("expected 2, got %d: %v", len(checks), checks)
	}
	if checks[0] != "count" || checks[1] != "render" {
		t.Errorf("unexpected order: %v", checks)
	}
}

func TestParseChecksSingleItem(t *testing.T) {
	checks, err := parseChecks("sort")
	if err != nil {
		t.Fatalf("parseChecks single: %v", err)
	}
	if len(checks) != 1 || checks[0] != "sort" {
		t.Errorf("expected [sort], got %v", checks)
	}
}

func TestParseChecksDeduplication(t *testing.T) {
	checks, err := parseChecks("count,count,sort")
	if err != nil {
		t.Fatalf("parseChecks dedup: %v", err)
	}
	if len(checks) != 2 {
		t.Errorf("expected 2 (deduplicated), got %d: %v", len(checks), checks)
	}
}

func TestParseChecksUnknownName(t *testing.T) {
	_, err := parseChecks("count,unknown")
	if err == nil {
		t.Error("expected error for unknown check name, got nil")
	}
}

func TestParseChecksEmpty(t *testing.T) {
	_, err := parseChecks("")
	if err == nil {
		t.Error("expected error for empty checks, got nil")
	}
}

func TestParseChecksWhitespace(t *testing.T) {
	checks, err := parseChecks(" count , sort ")
	if err != nil {
		t.Fatalf("parseChecks whitespace: %v", err)
	}
	if len(checks) != 2 || checks[0] != "count" || checks[1] != "sort" {
		t.Errorf("unexpected: %v", checks)
	}
}

// ── check selection logic ─────────────────────────────────────────────────────

func TestContainsStr(t *testing.T) {
	tests := []struct {
		slice []string
		s     string
		want  bool
	}{
		{[]string{"a", "b", "c"}, "b", true},
		{[]string{"a", "b", "c"}, "d", false},
		{nil, "a", false},
		{[]string{}, "a", false},
	}
	for _, tc := range tests {
		got := containsStr(tc.slice, tc.s)
		if got != tc.want {
			t.Errorf("containsStr(%v, %q) = %v, want %v", tc.slice, tc.s, got, tc.want)
		}
	}
}

// ── JSON output format ────────────────────────────────────────────────────────

func TestJSONOutputFormat(t *testing.T) {
	results := []CheckResult{
		{Name: "count", Status: "PASS", Detail: "all match"},
		{Name: "sort", Status: "FAIL", Detail: "diverge at index 0"},
	}

	// Write to a temp file and read back
	tf, err := os.CreateTemp(t.TempDir(), "smoke-test-*.json")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer tf.Close()

	emitJSON(tf, "/fake/dir", results, "FAIL")
	_ = tf.Sync()

	data, err := os.ReadFile(tf.Name())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var report jsonReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("Unmarshal: %v — raw: %s", err, data)
	}

	if report.Dir != "/fake/dir" {
		t.Errorf("dir: want /fake/dir, got %q", report.Dir)
	}
	if report.Result != "FAIL" {
		t.Errorf("result: want FAIL, got %q", report.Result)
	}
	if len(report.Checks) != 2 {
		t.Errorf("checks: want 2, got %d", len(report.Checks))
	}
	if report.Checks[0].Name != "count" || report.Checks[0].Status != "PASS" {
		t.Errorf("checks[0]: %+v", report.Checks[0])
	}
	if report.Checks[1].Name != "sort" || report.Checks[1].Status != "FAIL" {
		t.Errorf("checks[1]: %+v", report.Checks[1])
	}
}

func TestJSONOutputIsValidJSON(t *testing.T) {
	// Verify the JSON emitter produces parseable output even with special chars.
	results := []CheckResult{
		{Name: "search", Status: "PASS", Detail: `[text=render] gw=5 bd=5 ok`},
	}

	tf, err := os.CreateTemp(t.TempDir(), "smoke-test-*.json")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer tf.Close()

	emitJSON(tf, "/path/with spaces", results, "PASS")
	_ = tf.Sync()

	data, err := os.ReadFile(tf.Name())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Must be valid JSON
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Errorf("JSON is not valid: %v\nraw: %s", err, data)
	}
}

// ── render check (unit, no repository) ──────────────────────────────────────────

func TestRenderCheckPassesWithFakeBoard(t *testing.T) {
	result := runRenderCheck()
	if result.Name != "render" {
		t.Errorf("name: want render, got %q", result.Name)
	}
	if result.Status != "PASS" {
		t.Errorf("status: want PASS, got %q — detail: %s", result.Status, result.Detail)
	}
}

func TestCountColumnTopBorders(t *testing.T) {
	tests := []struct {
		view string
		want int
	}{
		{"no borders here", 0},
		{"╭─── one", 1},
		{"╭ ╭ ╭ ╭ four", 4},
		{"╭\n╭\n╭\n╭\n", 4},
	}
	for _, tc := range tests {
		got := countColumnTopBorders(tc.view)
		if got != tc.want {
			t.Errorf("countColumnTopBorders(%q) = %d, want %d", tc.view, got, tc.want)
		}
	}
}
