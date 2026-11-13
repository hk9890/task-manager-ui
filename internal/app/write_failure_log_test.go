package app

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	issueeditmode "github.com/hk9890/task-manager-ui/internal/mode/issueedit"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
)

// A save the store refuses leaves its cause in the log, which the editor's
// error line leaves out: for a busy store, the store and the wait.
func TestARefusedSaveLogsTheWriteFailureWithItsCause(t *testing.T) {
	gw := fakes.NewTracked()
	seedReady(gw, "tm-1", "Ready first", "task", 1)
	services, err := NewServices(gw, config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices returned error: %v", err)
	}
	logs := &bytes.Buffer{}
	services.Logger = slog.New(slog.NewTextHandler(logs, nil))

	m := mustNewModel(t, services)
	m = applyMessages(t, m, runBatch(m.Init()))
	m = withEditorOn(m, issueeditmode.KindEdit, "tm-1")

	busy := domain.RepositoryError{
		Code:      domain.ErrorCodeTimeout,
		Operation: "update issue",
		Message:   "store busy: another write holds the lock; try again",
		Cause:     errors.New("store busy: /stores/demo: another writer held the write lock for 10s"),
	}
	next, _ := m.Update(editorSavedMsg{err: busy})
	_ = next.(Model)

	record := logs.String()
	for _, want := range []string{"issue write failed", "issue=tm-1", "/stores/demo", "held the write lock for 10s"} {
		if !strings.Contains(record, want) {
			t.Errorf("log lacks %q:\n%s", want, record)
		}
	}
}

// An error that is no repository error did not come from a write, so it is
// not recorded as one.
func TestAnErrorThatIsNoRepositoryErrorIsNoWriteFailure(t *testing.T) {
	logs := &bytes.Buffer{}
	m := Model{services: Services{Logger: slog.New(slog.NewTextHandler(logs, nil))}}

	m.logWriteFailure("tm-1", errors.New("the agent host reported no session"))

	if logs.Len() != 0 {
		t.Errorf("logged %q, want nothing", logs.String())
	}
}
