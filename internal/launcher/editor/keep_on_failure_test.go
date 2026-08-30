package editor_test

import (
	"context"
	"os"
	"strings"
	"testing"

	launchereditor "github.com/hk9890/task-manager-ui/internal/launcher/editor"
	memoryrepo "github.com/hk9890/task-manager-ui/internal/repository/memory"
)

// TestApplyEditsKeepsTheDocumentWhenItDoesNotParse pins the operator's work
// against a parse failure.
//
// ApplyEdits removed the temp file on every path, so a document the parser
// rejected was deleted with the operator's rewritten description in it and only
// a marker name in the toast. The field scanner reads the blocks in document
// order, so simply moving one editable block above another is enough to reach
// this path.
func TestApplyEditsKeepsTheDocumentWhenItDoesNotParse(t *testing.T) {
	t.Parallel()

	repo := memoryrepo.New()
	repo.Seed(memoryrepo.Issue{ID: "tm-7", Title: "Old", Status: "open", Type: "task", Priority: 2})

	service, err := launchereditor.NewIssueEditor(repo, "vi")
	if err != nil {
		t.Fatalf("NewIssueEditor: %v", err)
	}

	prepared, err := service.PrepareDocument(context.Background(), "tm-7")
	if err != nil {
		t.Fatalf("PrepareDocument: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(prepared.TempPath) })

	const rewritten = "the description the operator spent ten minutes on"
	if err := os.WriteFile(prepared.TempPath, []byte("garbage without any markers\n\n"+rewritten+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = service.ApplyEdits(context.Background(), "tm-7", prepared.Issue, prepared.TempPath)
	if err == nil {
		t.Fatal("ApplyEdits: want a parse error for a document with no markers")
	}
	if !strings.HasPrefix(err.Error(), "edits kept at "+prepared.TempPath) {
		t.Errorf("the error must lead with the kept file's path — a toast is clipped to the terminal width: %v", err)
	}

	content, readErr := os.ReadFile(prepared.TempPath)
	if readErr != nil {
		t.Fatalf("the edited document was deleted on a parse failure: %v", readErr)
	}
	if !strings.Contains(string(content), rewritten) {
		t.Errorf("the kept file lost the operator's text: %q", content)
	}
}

// TestApplyEditsRemovesTheDocumentOnEverySuccessPath pins the other side: a
// document that has been dealt with is not left behind.
func TestApplyEditsRemovesTheDocumentOnEverySuccessPath(t *testing.T) {
	t.Parallel()

	cases := map[string]func(original string) string{
		"saved":     func(original string) string { return strings.Replace(original, "Old", "New", 1) },
		"unchanged": func(original string) string { return original },
	}

	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			repo := memoryrepo.New()
			repo.Seed(memoryrepo.Issue{ID: "tm-7", Title: "Old", Status: "open", Type: "task", Priority: 2})

			service, err := launchereditor.NewIssueEditor(repo, "vi")
			if err != nil {
				t.Fatalf("NewIssueEditor: %v", err)
			}
			prepared, err := service.PrepareDocument(context.Background(), "tm-7")
			if err != nil {
				t.Fatalf("PrepareDocument: %v", err)
			}

			original, err := os.ReadFile(prepared.TempPath)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if err := os.WriteFile(prepared.TempPath, []byte(edit(string(original))), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			if _, err := service.ApplyEdits(context.Background(), "tm-7", prepared.Issue, prepared.TempPath); err != nil {
				t.Fatalf("ApplyEdits: %v", err)
			}
			if _, err := os.Stat(prepared.TempPath); !os.IsNotExist(err) {
				t.Errorf("temp document still present after a %s apply", name)
			}
		})
	}
}
