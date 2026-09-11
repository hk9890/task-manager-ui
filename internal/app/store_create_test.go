package app

// Creating a store from the picker: when it is offered, what the form asks,
// and what happens when creation succeeds or fails.

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/mode"
	storepickermode "github.com/hk9890/task-manager-ui/internal/mode/storepicker"
	"github.com/hk9890/task-manager-ui/internal/repository/nostore"
	"github.com/hk9890/task-manager-ui/internal/storecatalog"
	"github.com/hk9890/task-manager-ui/internal/testing/fakes"
	testui "github.com/hk9890/task-manager-ui/internal/testing/ui"
	"github.com/hk9890/task-manager-ui/internal/ui/modal"
)

const storelessDir = "/home/hans/dev/widget"

type storelessStart struct {
	m       Model
	catalog *fakes.FakeStoreCatalog
}

// newStorelessStart is a start in a directory with no store, where the picker
// offers to create one. The catalog hands back a store with one issue for any
// create, so a successful create shows on the board.
func newStorelessStart(t *testing.T) storelessStart {
	t.Helper()

	created := fakes.NewTracked()
	seedReady(created, "wdg-1", "Widget store issue", "task", 1)
	catalog := &fakes.FakeStoreCatalog{
		Entries: []storecatalog.Entry{
			{Name: "bravo", ProjectPath: "/home/hans/dev/bravo", StorePath: "/stores/bravo", Health: storecatalog.HealthOK},
		},
		Created: storecatalog.Opened{
			Repo: created, Name: "widget", ProjectPath: t.TempDir(), StorePath: storelessDir + "/.tasks",
		},
	}

	services, err := NewServices(nostore.New(), config.Default(), t.TempDir())
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	services.StoreCatalog = catalog

	m := mustNewModelWithOptions(t, services, RuntimeOptions{
		UnresolvedStore: "No task-manager store for " + storelessDir,
		StorelessDir:    storelessDir,
	})
	m = applyMessages(t, m, runBatch(m.Init()))
	return storelessStart{m: m, catalog: catalog}
}

// openForm moves down rows and presses Enter on a create row, stepping the
// flow by hand: the form's modal schedules a repeating cursor tick, so draining
// its commands the way press does never returns (docs/TESTING.md).
func openForm(t *testing.T, m Model, rows int) Model {
	t.Helper()
	for i := 0; i < rows; i++ {
		m = press(t, m, "j")
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("Enter on a create row produced no command")
	}
	create, ok := cmd().(storepickermode.CreateMsg)
	if !ok {
		t.Fatal("Enter on a create row did not ask to create a store")
	}
	next, _ = m.Update(create) // the form's Init: the cursor tick, left unrun
	return next.(Model)
}

func submit(t *testing.T, m Model, values map[string]string) Model {
	t.Helper()
	next, cmd := m.Update(modal.SubmitMsg{Values: values})
	return applyMessages(t, next.(Model), runBatch(cmd))
}

func TestStorelessStartOffersToCreateAStoreThere(t *testing.T) {
	s := newStorelessStart(t)

	view := pickerView(s.m)
	for _, want := range []string{"Create a local store in " + storelessDir, "Create a central store for " + storelessDir, "bravo"} {
		if !strings.Contains(view, want) {
			t.Errorf("expected %q on the picker:\n%s", want, view)
		}
	}
}

// With a store resolved there is nothing to create; with an unregistered
// --store-name the working directory may have a store of its own. Neither
// offers creation.
func TestCreationIsNotOfferedWithoutAStorelessDirectory(t *testing.T) {
	cases := map[string]RuntimeOptions{
		"store resolved":          {},
		"unregistered store name": {UnresolvedStore: `No central store is registered as "nope"`},
	}
	for name, runtime := range cases {
		t.Run(name, func(t *testing.T) {
			services, err := NewServices(fakes.NewTracked(), config.Default(), t.TempDir())
			if err != nil {
				t.Fatalf("NewServices: %v", err)
			}
			services.StoreCatalog = &fakes.FakeStoreCatalog{}

			m := mustNewModelWithOptions(t, services, runtime)
			m = applyMessages(t, m, runBatch(m.Init()))
			if m.active != mode.StorePicker {
				m = press(t, m, "s")
			}

			if strings.Contains(pickerView(m), "Create a") {
				t.Errorf("creation offered where it must not be:\n%s", pickerView(m))
			}
		})
	}
}

func TestCreatingALocalStoreOpensIt(t *testing.T) {
	s := newStorelessStart(t)

	m := openForm(t, s.m, 0) // the first row: create a local store
	if !m.showActionModal {
		t.Fatal("Enter on the create row did not open the form")
	}
	form := pickerView(m)
	if !strings.Contains(form, "Create Local Store") || !strings.Contains(form, "pfx") {
		t.Errorf("expected the local-store form with the derived prefix prefilled:\n%s", form)
	}

	m = submit(t, m, map[string]string{"prefix": "wdg"})

	creates := s.catalog.Creates()
	if len(creates) != 1 || creates[0].Central || creates[0].Dir != storelessDir || creates[0].Prefix != "wdg" {
		t.Fatalf("creates: got %+v, want one local store in %s with prefix wdg", creates, storelessDir)
	}
	if m.showActionModal {
		t.Error("the form is still open after the store was created")
	}
	if m.active != mode.Board {
		t.Fatalf("active mode after creating: got %q, want the new store's board", m.active)
	}
	if !strings.Contains(pickerView(m), "Widget store issue") {
		t.Errorf("the board does not show the created store:\n%s", pickerView(m))
	}

	// The directory has a store now; the picker stops offering to create one.
	m = press(t, m, "s")
	if strings.Contains(pickerView(m), "Create a") {
		t.Errorf("creation is still offered after the store was created:\n%s", pickerView(m))
	}
}

func TestCreatingACentralStoreSendsItsName(t *testing.T) {
	s := newStorelessStart(t)

	m := openForm(t, s.m, 1) // the second row: create a central store
	form := pickerView(m)
	if !strings.Contains(form, "Create Central Store") || !strings.Contains(form, "widget") || !strings.Contains(form, "pfx") {
		t.Errorf("expected the central-store form with the directory name and derived prefix prefilled:\n%s", form)
	}

	submit(t, m, map[string]string{"name": "widget-tracker", "prefix": "wdg"})

	creates := s.catalog.Creates()
	if len(creates) != 1 || !creates[0].Central || creates[0].Name != "widget-tracker" || creates[0].Dir != storelessDir {
		t.Fatalf("creates: got %+v, want one central store widget-tracker for %s", creates, storelessDir)
	}
}

// A rejected create — a name already taken — keeps the form open with what was
// typed, and says why. Retyping a form to fix one field is the cost this avoids.
func TestAFailedCreateKeepsTheFormOpen(t *testing.T) {
	s := newStorelessStart(t)
	s.catalog.CreateErr = errors.New(`central store "widget" already exists`)

	m := openForm(t, s.m, 1)
	m = submit(t, m, map[string]string{"name": "widget", "prefix": "wdg"})

	if !m.showActionModal || m.storeForm.kind != storepickermode.CentralStore {
		t.Fatal("the form closed after a failed create")
	}
	if m.storeForm.creating {
		t.Error("the form is stuck in its creating state after the failure arrived")
	}
	if !m.toast.Visible() || !strings.Contains(m.toast.View(), "already exists") {
		t.Errorf("expected a toast naming the collision, got %q", m.toast.View())
	}
	if m.active != mode.StorePicker {
		t.Errorf("active mode: got %q, want the picker", m.active)
	}
	// The same form, with what it held, not a fresh one.
	if form := pickerView(m); !strings.Contains(form, "Create Central Store") || !strings.Contains(form, "widget") {
		t.Errorf("the form lost its inputs after the failure:\n%s", form)
	}
}

// The form over the picker, in both shapes. Width is fixed because it decides
// where the modal lands over the picker behind it.
func TestStoreFormGoldens(t *testing.T) {
	for _, tc := range []struct {
		rows   int
		golden string
	}{
		{0, "store_form_local_w100.golden"},
		{1, "store_form_central_w100.golden"},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			s := newStorelessStart(t)
			next, _ := s.m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
			m := openForm(t, next.(Model), tc.rows)
			m.toast = m.toast.Hide() // the start's "no store" toast, not the form under test

			testui.AssertMatchesGoldenNormalized(t, []byte(m.View()), tc.golden)
		})
	}
}

func TestAnEmptyPrefixIsRejectedBeforeCreating(t *testing.T) {
	s := newStorelessStart(t)

	m := openForm(t, s.m, 0)
	m = submit(t, m, map[string]string{"prefix": "   "})

	if got := s.catalog.Creates(); len(got) != 0 {
		t.Errorf("an empty prefix reached the catalog: %+v", got)
	}
	if !m.showActionModal {
		t.Error("the form closed on a rejected prefix")
	}
	if !m.toast.Visible() || !strings.Contains(m.toast.View(), "prefix") {
		t.Errorf("expected a toast asking for a prefix, got %q", m.toast.View())
	}
}

func TestAnEmptyCentralNameIsRejectedBeforeCreating(t *testing.T) {
	s := newStorelessStart(t)

	m := openForm(t, s.m, 1)
	m = submit(t, m, map[string]string{"name": "", "prefix": "wdg"})

	if got := s.catalog.Creates(); len(got) != 0 {
		t.Errorf("an empty name reached the catalog: %+v", got)
	}
	if !m.showActionModal {
		t.Error("the form closed on a rejected name")
	}
}

// A second submit while the first create is still running must not start a
// second one: it could only fail on the store the first just made.
func TestASecondSubmitWhileCreatingIsIgnored(t *testing.T) {
	s := newStorelessStart(t)

	m := openForm(t, s.m, 0)
	next, first := m.Update(modal.SubmitMsg{Values: map[string]string{"prefix": "wdg"}})
	m = next.(Model)
	_, second := m.Update(modal.SubmitMsg{Values: map[string]string{"prefix": "wdg"}})

	if second != nil {
		runBatch(second)
	}
	runBatch(first)
	if got := s.catalog.Creates(); len(got) != 1 {
		t.Errorf("the catalog was asked to create %d stores, want 1", len(got))
	}
}

// The created store arrives while its form is still open. An open overlay
// swallows messages that are not the shell's own, so this one must be exempt.
func TestTheCreatedStoreIsNotSwallowedByItsOwnForm(t *testing.T) {
	s := newStorelessStart(t)

	m := openForm(t, s.m, 0)
	next, cmd := m.Update(modal.SubmitMsg{Values: map[string]string{"prefix": "wdg"}})
	m = next.(Model)
	if !m.showActionModal {
		t.Fatal("fixture: the form must still be open while the store is created")
	}

	var created tea.Msg
	for _, msg := range runBatch(cmd) {
		if _, ok := msg.(storeCreatedMsg); ok {
			created = msg
		}
	}
	if created == nil {
		t.Fatal("submitting the form produced no storeCreatedMsg")
	}

	next, _ = m.Update(created)
	if next.(Model).showActionModal {
		t.Error("the form's overlay swallowed the created store")
	}
}
