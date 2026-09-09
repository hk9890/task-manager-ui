// Package storepicker is the store-picker controller: the full-screen surface
// listing every central task-manager store on this machine.
//
// It is not a browse tab. The header strip is the three browse modes and the
// picker sits above all of them, so it is absent from mode.BrowseModes and
// renders instead of the shell rather than inside it (docs/DESIGN-GUIDE.md).
package storepicker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/storecatalog"
	"github.com/hk9890/task-manager-ui/internal/ui/scroll"
	uistorepicker "github.com/hk9890/task-manager-ui/internal/ui/storepicker"
)

// defaultItemCapacity is the row window used before the first
// tea.WindowSizeMsg sets a real height.
const defaultItemCapacity = 20

// StoresLoadedMsg carries the result of a catalog listing. It is exported
// because the shell arms the reload that produces it.
type StoresLoadedMsg struct {
	Entries []storecatalog.Entry
	Err     error
}

// Model is the store-picker controller.
type Model struct {
	ctx     context.Context
	catalog storecatalog.Catalog
	logger  *slog.Logger
	keys    config.ResolvedKeyBindings

	width  int
	height int

	entries []storecatalog.Entry
	err     error

	// activeStorePath is the store directory the app is currently browsing. It
	// marks one row as the active store; empty when no store is open.
	activeStorePath string

	// loading is the visual loading state; inflight guards concurrent reloads.
	loading  bool
	inflight bool

	selectedRow  int
	scrollOffset int
}

// NewModel builds the store-picker controller. Keybindings default to the
// resolved defaults when no resolved set is supplied.
func NewModel(ctx context.Context, catalog storecatalog.Catalog, logger *slog.Logger, resolved ...config.ResolvedKeyBindings) *Model {
	if logger == nil {
		logger = slog.Default()
	}
	var keys config.ResolvedKeyBindings
	if len(resolved) > 0 {
		keys = resolved[0]
	} else {
		var err error
		keys, err = config.ResolveKeyBindings(config.DefaultKeyBindings())
		if err != nil {
			panic(fmt.Sprintf("invalid default store-picker keybindings: %v", err))
		}
	}

	return &Model{
		ctx:     ctx,
		catalog: catalog,
		logger:  logger,
		keys:    keys,
	}
}

// Init loads the store list.
func (m *Model) Init() tea.Cmd {
	return m.startReload()
}

// Update processes picker messages other than keys.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
	case StoresLoadedMsg:
		m.apply(msg)
	}
	return nil
}

// HandleKey processes one key press and reports whether the picker consumed it.
// An unconsumed key falls through to the shell, which is how Escape, quit and
// help keep working while the picker is up.
//
// Row movement and reload read the board keybinding context: the picker is a
// single scrolling list of rows, the same shape a board column is, and a
// context of its own would ask the operator to rebind the same movement twice.
func (m *Model) HandleKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch {
	case m.keys.Match(config.BoardContext, config.BoardActionMoveUp, msg):
		m.moveRow(-1)
		return true, nil
	case m.keys.Match(config.BoardContext, config.BoardActionMoveDown, msg):
		m.moveRow(1)
		return true, nil
	case m.keys.Match(config.BoardContext, config.BoardActionReload, msg):
		return true, m.startReload()
	}
	return false, nil
}

// View renders the picker full screen.
func (m *Model) View(spinnerFrame int, help string) string {
	errText := ""
	if m.err != nil {
		errText = m.err.Error()
	}

	rows := make([]uistorepicker.Row, 0, len(m.entries))
	for _, entry := range m.entries {
		rows = append(rows, uistorepicker.Row{
			Name:        entry.Name,
			ProjectPath: entry.ProjectPath,
			Health:      string(entry.Health),
			Usable:      entry.Health.Usable(),
			Active:      entry.StorePath != "" && entry.StorePath == m.activeStorePath,
		})
	}

	return uistorepicker.Render(uistorepicker.State{
		Rows:         rows,
		SelectedRow:  m.selectedRow,
		ScrollOffset: m.scrollOffset,
		Loading:      m.loading,
		Error:        errText,
		Help:         help,
		SpinnerFrame: spinnerFrame,
		Width:        m.width,
		Height:       m.height,
	})
}

// SetSize updates render dimensions. The clamp mirrors the board's: the row
// window is derived from the height, so a resize leaves an offset that was
// valid only for the old window.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.clampSelection()
}

// SetActiveStorePath marks the row for the store the app is browsing. An empty
// path marks none, which is what the no-store start looks like.
func (m *Model) SetActiveStorePath(storePath string) {
	m.activeStorePath = storePath
}

// IsLoading reports whether a listing is in flight.
func (m *Model) IsLoading() bool {
	return m.loading
}

// Entries returns the listed stores, in registry order.
func (m *Model) Entries() []storecatalog.Entry {
	out := make([]storecatalog.Entry, len(m.entries))
	copy(out, m.entries)
	return out
}

// SelectedEntry returns the highlighted store, or false when the list is empty.
func (m *Model) SelectedEntry() (storecatalog.Entry, bool) {
	if len(m.entries) == 0 {
		return storecatalog.Entry{}, false
	}
	row := m.selectedRow
	if row < 0 || row >= len(m.entries) {
		row = 0
	}
	return m.entries[row], true
}

func (m *Model) startReload() tea.Cmd {
	if m.inflight {
		m.logger.Debug("startReload re-entry suppressed; store listing already in flight")
		return nil
	}
	m.inflight = true
	m.loading = true
	m.err = nil
	return loadStoresCmd(m.ctx, m.catalog)
}

func (m *Model) apply(msg StoresLoadedMsg) {
	m.loading = false
	m.inflight = false
	m.err = msg.Err

	if msg.Err != nil {
		m.logger.Error("failed to list central task-manager stores", "error", msg.Err)
		// Keep the stale rows on screen; the inline error row says why they may
		// be out of date.
		return
	}

	m.entries = msg.Entries
	m.clampSelection()
}

func (m *Model) clampSelection() {
	if len(m.entries) == 0 {
		m.selectedRow = 0
		m.scrollOffset = 0
		return
	}
	if m.selectedRow < 0 {
		m.selectedRow = 0
	}
	if m.selectedRow >= len(m.entries) {
		m.selectedRow = len(m.entries) - 1
	}
	capacity := m.itemCapacity()
	if maxOffset := len(m.entries) - capacity; m.scrollOffset > maxOffset {
		m.scrollOffset = max(maxOffset, 0)
	}
	m.scrollOffset = scroll.EnsureVisible(m.scrollOffset, m.selectedRow, capacity)
}

func (m *Model) moveRow(delta int) {
	if len(m.entries) == 0 {
		m.selectedRow = 0
		return
	}
	m.selectedRow += delta
	m.clampSelection()
}

// itemCapacity returns the number of store rows that fit at the current
// height. It mirrors what the renderer draws, so the scroll window and the
// visible rows cannot disagree.
func (m *Model) itemCapacity() int {
	if m.height == 0 {
		return defaultItemCapacity
	}
	return uistorepicker.RowCapacity(m.height, m.err != nil)
}

func loadStoresCmd(ctx context.Context, catalog storecatalog.Catalog) tea.Cmd {
	return func() tea.Msg {
		// A programmatic embed can build the shell without a catalog. Report
		// that on the picker rather than panicking on the first listing.
		if catalog == nil {
			return StoresLoadedMsg{Err: errors.New("no store catalog is configured for this session")}
		}
		entries, err := catalog.Stores(ctx)
		return StoresLoadedMsg{Entries: entries, Err: err}
	}
}
