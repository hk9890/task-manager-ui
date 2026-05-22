package fakes

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/beads-workbench/internal/domain"
	launchereditor "github.com/hk9890/beads-workbench/internal/launcher/editor"
)

// EditorCall captures one PrepareDocument invocation.
type EditorCall struct {
	IssueID string
}

// FakeEditor is a deterministic, non-interactive editor seam for tests.
//
// PrepareDocument records the call and returns a Prepared value that the model
// can pass to ApplyEdits. By default (no PrepareErr, no PrepareIssue override)
// it returns a Prepared with TempPath set to a sentinel value ("fake-path")
// and an empty Issue — tests that care about the apply result should also set
// ApplyResult/ApplyErr.
//
// BuildEditorCmd returns a no-op true command so tests that happen to call it
// don't spawn real editors.
type FakeEditor struct {
	mu sync.Mutex

	// PrepareErr, when non-nil, is returned by PrepareDocument.
	PrepareErr error
	// PrepareIssue, when non-zero, is returned as Prepared.Issue.
	PrepareIssue domain.IssueDetail

	// ApplyResult is returned by ApplyEdits when ApplyErr is nil.
	ApplyResult launchereditor.Result
	// ApplyErr, when non-nil, is returned by ApplyEdits.
	ApplyErr error

	// Calls records each PrepareDocument invocation (IssueID).
	Calls []EditorCall
}

var _ launchereditor.Service = (*FakeEditor)(nil)

// PrepareDocument records the call and returns configured results.
func (f *FakeEditor) PrepareDocument(_ context.Context, issueID string) (launchereditor.Prepared, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Calls = append(f.Calls, EditorCall{IssueID: issueID})
	if f.PrepareErr != nil {
		return launchereditor.Prepared{}, f.PrepareErr
	}

	return launchereditor.Prepared{
		IssueID:  issueID,
		Issue:    f.PrepareIssue,
		TempPath: "fake-path",
	}, nil
}

// ApplyEdits returns the configured result/error without touching the filesystem.
func (f *FakeEditor) ApplyEdits(_ context.Context, _ string, _ domain.IssueDetail, _ string) (launchereditor.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ApplyErr != nil {
		return launchereditor.Result{}, f.ApplyErr
	}
	return f.ApplyResult, nil
}

// BuildEditorCmd returns a no-op command ("true") so tests that call it
// don't accidentally spawn interactive processes.
func (f *FakeEditor) BuildEditorCmd(_ string) (*exec.Cmd, error) {
	cmd := exec.Command("true")
	if cmd == nil {
		return nil, fmt.Errorf("fake: could not build no-op editor command")
	}
	return cmd, nil
}

// FakeExecCommand satisfies tea.ExecCommand for tests. It intercepts the
// editor subprocess launch that the model routes through tea.Exec.
//
// Usage:
//
//	cmd := &fakes.FakeExecCommand{EditedContent: "...modified doc..."}
//	services.ExecCommandFactory = cmd.Factory()
//	// ... run test ...
//	if cmd.RunCalled != 1 { t.Fatalf(...) }
//
// Factory captures the temp-file path from the *exec.Cmd Args slice (last
// element) so Run() knows where to write EditedContent. SetStdin, SetStdout,
// and SetStderr are no-ops — Bubble Tea calls all three before Run.
type FakeExecCommand struct {
	mu sync.Mutex

	// EditedContent is written to the temp-file path on Run(). When empty,
	// Run() leaves the file contents unchanged.
	EditedContent string

	// RunErr is returned by Run(). nil means success.
	RunErr error

	// RunCalled counts how many times Run() has been called. Inspect after
	// the program settles.
	RunCalled int

	// path is captured from the *exec.Cmd when the factory is called.
	path string
}

var _ tea.ExecCommand = (*FakeExecCommand)(nil)

// Factory returns the ExecCommandFactory function to inject into Services.
// The returned factory captures the temp-file path from cmd.Args and ties it
// to this FakeExecCommand instance.
func (f *FakeExecCommand) Factory() func(*exec.Cmd) tea.ExecCommand {
	return func(cmd *exec.Cmd) tea.ExecCommand {
		f.mu.Lock()
		defer f.mu.Unlock()
		// The last argument of the built editor command is the temp-file path.
		if len(cmd.Args) > 0 {
			f.path = cmd.Args[len(cmd.Args)-1]
		}
		return f
	}
}

// Run writes EditedContent to the captured temp path (when non-empty) and
// returns RunErr. Bubble Tea calls SetStdin/SetStdout/SetStderr before Run.
func (f *FakeExecCommand) Run() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.RunCalled++
	if f.EditedContent != "" && f.path != "" {
		if err := os.WriteFile(f.path, []byte(f.EditedContent), 0o600); err != nil {
			return fmt.Errorf("FakeExecCommand: write edited content: %w", err)
		}
	}
	return f.RunErr
}

// RunCount returns the number of times Run() has been called. It is safe to
// call from any goroutine, including a WaitForConditionWithTimeout closure.
func (f *FakeExecCommand) RunCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.RunCalled
}

// SetStdin is a no-op; Bubble Tea calls it before Run.
func (f *FakeExecCommand) SetStdin(_ io.Reader) {}

// SetStdout is a no-op; Bubble Tea calls it before Run.
func (f *FakeExecCommand) SetStdout(_ io.Writer) {}

// SetStderr is a no-op; Bubble Tea calls it before Run.
func (f *FakeExecCommand) SetStderr(_ io.Writer) {}
