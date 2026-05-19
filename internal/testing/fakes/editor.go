package fakes

import (
	"context"
	"sync"

	launchereditor "github.com/hk9890/beads-workbench/internal/launcher/editor"
)

// EditorCall captures one editor launch invocation.
type EditorCall struct {
	IssueID string
}

// FakeEditor is a deterministic, non-interactive editor seam for tests.
type FakeEditor struct {
	mu sync.Mutex

	Result launchereditor.Result
	Err    error
	Calls  []EditorCall
}

var _ launchereditor.Service = (*FakeEditor)(nil)

// EditIssue records the call and returns configured results.
func (f *FakeEditor) EditIssue(_ context.Context, issueID string) (launchereditor.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, EditorCall{IssueID: issueID})
	if f.Err != nil {
		return launchereditor.Result{}, f.Err
	}

	return f.Result, nil
}
