package taskmgr

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hk9890/task-manager/sdk/tasks"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// TestMapReadErr drives every branch of mapReadErr — the normalizer for the
// non-ID reads (Dashboard, Search, Catalogs, HealthCheck). The "no store" branch
// in particular backs the actionable startup screen a user hits when launching in
// a directory without a .tasks store, so the SDK-sentinel-to-domain-code mapping
// is verified end to end here rather than only against a hand-built error.
func TestMapReadErr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   error
		want domain.ErrorCode
	}{
		{"no store", tasks.ErrNoStore, domain.ErrorCodeNoDatabaseFound},
		{"validation", &tasks.ValidationError{Field: "title", Message: "must not be empty"}, domain.ErrorCodeValidationFailed},
		{"parse", &tasks.ParseError{Pos: 3, Message: "bad filter expression"}, domain.ErrorCodeValidationFailed},
		{"unknown", errors.New("boom"), domain.ErrorCodeUnknown},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := mapReadErr("Dashboard", tc.in)

			var re domain.RepositoryError
			if !errors.As(got, &re) {
				t.Fatalf("mapReadErr returned %T (%v), want domain.RepositoryError", got, got)
			}
			if re.Code != tc.want {
				t.Errorf("code = %q, want %q", re.Code, tc.want)
			}
			if re.Operation != "Dashboard" {
				t.Errorf("operation = %q, want Dashboard", re.Operation)
			}
			if !errors.Is(got, tc.in) {
				t.Errorf("mapped error does not wrap the original cause %v", tc.in)
			}
		})
	}
}

// TestMapReadErrNil confirms mapReadErr passes a nil error through unchanged.
func TestMapReadErrNil(t *testing.T) {
	t.Parallel()
	if got := mapReadErr("Dashboard", nil); got != nil {
		t.Fatalf("mapReadErr(nil) = %v, want nil", got)
	}
}

// TestMapWriteErr drives every branch of mapWriteErr — the normalizer for the
// write methods (CreateIssue, UpdateIssue, CloseIssue, AddComment). The
// ErrNoStore branch in particular is otherwise unexercised by the suite.
func TestMapWriteErr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   error
		want domain.ErrorCode
	}{
		{"not found", tasks.ErrNotFound, domain.ErrorCodeCommandFailed},
		{"validation", &tasks.ValidationError{Field: "title", Message: "must not be empty"}, domain.ErrorCodeValidationFailed},
		{"immutable", tasks.ErrImmutable, domain.ErrorCodeConflict},
		{"no store", tasks.ErrNoStore, domain.ErrorCodeNoDatabaseFound},
		{"hook denied", &tasks.HookDeniedError{
			Event:   "pre-update",
			Hook:    "pkg:task-writing:body-sections",
			IssueID: "tm-1",
			Exit:    1,
			Reason:  "denied (exit 1)",
		}, domain.ErrorCodeHookDenied},
		{"unknown", errors.New("boom"), domain.ErrorCodeUnknown},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := mapWriteErr("UpdateIssue", tc.in)

			var re domain.RepositoryError
			if !errors.As(got, &re) {
				t.Fatalf("mapWriteErr returned %T (%v), want domain.RepositoryError", got, got)
			}
			if re.Code != tc.want {
				t.Errorf("code = %q, want %q", re.Code, tc.want)
			}
			if re.Operation != "UpdateIssue" {
				t.Errorf("operation = %q, want UpdateIssue", re.Operation)
			}
			if !errors.Is(got, tc.in) {
				t.Errorf("mapped error does not wrap the original cause %v", tc.in)
			}
		})
	}
}

// TestMapWriteErrHookDeniedKeepsTheHookReason pins what the operator reads when
// a lifecycle hook refuses a write. The rendered toast is the mapped error's
// own text, so a denial that falls through to the default branch reaches the
// screen as "unknown" — a policy refusal framed as a fault in the app. The
// assertion is on the message an operator sees, not only on the code.
func TestMapWriteErrHookDeniedKeepsTheHookReason(t *testing.T) {
	t.Parallel()

	denied := &tasks.HookDeniedError{
		Event:   "pre-close",
		Hook:    "pkg:deny-closes:deny-closes",
		IssueID: "tm-42",
		Exit:    1,
		Reason:  "refused by the machine-wide policy",
	}

	got := mapWriteErr("close issue", denied)

	text := got.Error()
	if strings.Contains(text, string(domain.ErrorCodeUnknown)) {
		t.Errorf("rendered error still frames the refusal as unknown: %q", text)
	}
	if !strings.Contains(text, denied.Hook) {
		t.Errorf("rendered error does not name the denying hook: %q", text)
	}
	if !strings.Contains(text, denied.Reason) {
		t.Errorf("rendered error does not carry the denial reason: %q", text)
	}
	// A toast is one line cut to the terminal width. With the hook id before
	// it, the reason was past the cut at 80 columns.
	if reason, hook := strings.Index(text, denied.Reason), strings.Index(text, denied.Hook); reason > hook {
		t.Errorf("the hook id stands before the denial reason, where a narrow toast cuts the reason: %q", text)
	}
}

// TestMappedErrorTextNamesTheSDKMessageOnce pins the text of the error toast.
// The mapped error carried the SDK message in its own message and again in the
// printed cause, so a refused title edit read "update: title is required:
// title: title is required". The SDK error must still be reachable: the text
// names it one time, the chain keeps it.
func TestMappedErrorTextNamesTheSDKMessageOnce(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mapped error
		once   string
		want   string
		found  func(error) bool
	}{
		{
			name:   "write validation",
			mapped: mapWriteErr("update", &tasks.ValidationError{Field: "title", Message: "title is required"}),
			once:   "title is required",
			want:   "update: title: title is required",
			found:  func(err error) bool { var target *tasks.ValidationError; return errors.As(err, &target) },
		},
		{
			name: "write hook denied",
			mapped: mapWriteErr("close issue", &tasks.HookDeniedError{
				Event:   "pre-close",
				Hook:    "pkg:deny-closes:deny-closes",
				IssueID: "tm-42",
				Exit:    1,
				Reason:  "refused by the machine-wide policy",
			}),
			once:  "refused by the machine-wide policy",
			want:  `close issue: refused by the machine-wide policy: pre-close denied for tm-42 by hook "pkg:deny-closes:deny-closes"`,
			found: func(err error) bool { var target *tasks.HookDeniedError; return errors.As(err, &target) },
		},
		{
			name:   "write validation the SDK wrapped",
			mapped: mapWriteErr("create issue", fmt.Errorf("entry 2: %w", &tasks.ValidationError{Field: "title", Message: "title is required"})),
			once:   "title is required",
			want:   "create issue: entry 2: title: title is required",
			found:  func(err error) bool { var target *tasks.ValidationError; return errors.As(err, &target) },
		},
		{
			name:   "read validation",
			mapped: mapReadErr("search", &tasks.ValidationError{Field: "statuses", Message: `unknown status "nope"`}),
			once:   `unknown status "nope"`,
			want:   `search: statuses: unknown status "nope"`,
			found:  func(err error) bool { var target *tasks.ValidationError; return errors.As(err, &target) },
		},
		{
			name:   "read parse",
			mapped: mapReadErr("search", &tasks.ParseError{Pos: 3, Message: "bad filter expression"}),
			once:   "bad filter expression",
			want:   "search: parse error at byte 3: bad filter expression",
			found:  func(err error) bool { var target *tasks.ParseError; return errors.As(err, &target) },
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			text := tc.mapped.Error()
			if got := strings.Count(text, tc.once); got != 1 {
				t.Errorf("text holds %q %d times, want 1: %q", tc.once, got, text)
			}
			if text != tc.want {
				t.Errorf("text = %q, want %q", text, tc.want)
			}
			if !tc.found(tc.mapped) {
				t.Errorf("errors.As does not find the SDK error in %T", tc.mapped)
			}
		})
	}
}

// TestMapWriteErrNil confirms mapWriteErr passes a nil error through unchanged.
func TestMapWriteErrNil(t *testing.T) {
	t.Parallel()
	if got := mapWriteErr("UpdateIssue", nil); got != nil {
		t.Fatalf("mapWriteErr(nil) = %v, want nil", got)
	}
}
