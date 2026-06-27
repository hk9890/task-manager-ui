package fakes

import (
	"context"
	"errors"
	"testing"
)

func TestFakeProcessRunnerRecordsLaunchIntent(t *testing.T) {
	t.Parallel()

	fake := &FakeProcessRunner{}
	err := fake.Run(context.Background(), "opencode", []string{"run", "--issue", "tm-10"}, "/tmp/work", []string{"EDITOR=nvim"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(fake.Calls) != 1 {
		t.Fatalf("expected one call, got %d", len(fake.Calls))
	}

	call := fake.Calls[0]
	if call.Command != "opencode" || len(call.Args) != 3 || call.Dir != "/tmp/work" {
		t.Fatalf("unexpected call record: %#v", call)
	}
}

func TestFakeProcessRunnerReturnsConfiguredError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("spawn failed")
	fake := &FakeProcessRunner{Err: wantErr}

	err := fake.Run(context.Background(), "nvim", []string{"/tmp/issue.md"}, "", nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected configured error, got %v", err)
	}
}
