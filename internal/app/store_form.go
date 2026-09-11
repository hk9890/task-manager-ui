package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	storepickermode "github.com/hk9890/task-manager-ui/internal/mode/storepicker"
	"github.com/hk9890/task-manager-ui/internal/storecatalog"
	"github.com/hk9890/task-manager-ui/internal/ui/modal"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

// storeForm is the create-a-store form while it is open. It rides the shell's
// action-modal slot, so it inherits that overlay's rendering, key capture and
// refresh suppression; kind is what tells a submit apart from a mutation's.
type storeForm struct {
	kind storepickermode.StoreKind // zero when no store form is open
	dir  string

	// creating is set while the store is being created. The form stays open
	// until creation succeeds, so a rejected name or prefix is corrected in
	// place instead of retyped, and a second submit meanwhile is ignored.
	creating bool
}

func (m *Model) openStoreForm(kind storepickermode.StoreKind, dir string) tea.Cmd {
	prefix := ""
	if m.services.StoreCatalog != nil {
		prefix = m.services.StoreCatalog.DerivePrefix(dir)
	}

	cfg := modal.Config{ConfirmText: "Create", MinWidth: 72}
	switch kind {
	case storepickermode.LocalStore:
		cfg.Title = "Create Local Store"
		cfg.Message = "Creates a .tasks store in " + dir
		cfg.Inputs = []modal.InputConfig{
			{Key: "prefix", Label: "ID prefix", Value: prefix},
		}
	case storepickermode.CentralStore:
		cfg.Title = "Create Central Store"
		cfg.Message = "Registers a central store for " + dir
		cfg.Inputs = []modal.InputConfig{
			{Key: "name", Label: "Name", Value: filepath.Base(dir)},
			{Key: "prefix", Label: "ID prefix", Value: prefix},
		}
	}

	m.storeForm = storeForm{kind: kind, dir: dir}
	m.actionModal = modal.NewWithKeys(cfg, modal.BindingsFromConfig(m.keys))
	m.actionModal.SetSize(m.width, m.height)
	m.showActionModal = true
	return m.actionModal.Init()
}

// submitStoreForm validates the form and starts creating the store. The form
// stays open either way; storeCreatedMsg closes it on success.
func (m *Model) submitStoreForm(values map[string]string) tea.Cmd {
	if m.storeForm.creating {
		return nil
	}

	name := strings.TrimSpace(values["name"])
	prefix := strings.TrimSpace(values["prefix"])
	if m.storeForm.kind == storepickermode.CentralStore && name == "" {
		return m.showToast("Enter a name for the central store", toaster.StyleWarn)
	}
	if prefix == "" {
		return m.showToast("Enter an ID prefix for the new store", toaster.StyleWarn)
	}

	m.storeForm.creating = true
	return createStoreCmd(m.appCtx, m.services.StoreCatalog, m.storeForm.kind, m.storeForm.dir, name, prefix)
}

// createStoreCmd creates a store. Like openStoreCmd it is not scoped: it
// decides which store is active rather than working inside one.
func createStoreCmd(ctx context.Context, catalog storecatalog.Catalog, kind storepickermode.StoreKind, dir, name, prefix string) tea.Cmd {
	return func() tea.Msg {
		if catalog == nil {
			return storeCreatedMsg{dir: dir, err: errors.New("no store catalog is configured for this session")}
		}
		var (
			opened storecatalog.Opened
			err    error
		)
		if kind == storepickermode.CentralStore {
			opened, err = catalog.CreateCentral(ctx, dir, name, prefix)
		} else {
			opened, err = catalog.CreateLocal(ctx, dir, prefix)
		}
		return storeCreatedMsg{dir: dir, opened: opened, err: err}
	}
}

func (m *Model) handleStoreCreated(msg storeCreatedMsg) tea.Cmd {
	m.storeForm.creating = false
	if msg.err != nil {
		m.logger().Error("failed to create task-manager store", "dir", msg.dir, "error", msg.err.Error())
		return m.showToast(fmt.Sprintf("Failed to create store: %v", msg.err), toaster.StyleError)
	}

	m.showActionModal = false
	m.storeForm = storeForm{}
	// The working directory has a store now; offering to create another would
	// only fail on the one just made.
	m.storePicker.SetCreateTarget("")
	return m.switchStore(msg.opened)
}
