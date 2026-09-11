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
	// Generation is the listing this result belongs to. A Bubble Tea command
	// cannot be cancelled, so a listing dispatched for an earlier open of the
	// picker still delivers after the picker has been closed and reopened;
	// comparing the generation is what drops it instead of rendering it.
	Generation int
}

// OpenMsg asks the shell to make Entry the active store. The picker emits it
// rather than opening the store itself: which store is active is the shell's
// state, and the picker only knows which row the operator chose.
type OpenMsg struct {
	Entry storecatalog.Entry
}

// StoreKind is the kind of store the picker offers to create.
type StoreKind int

const (
	// LocalStore is a .tasks directory inside the project.
	LocalStore StoreKind = iota + 1
	// CentralStore is a store under the central root, registered for the
	// project.
	CentralStore
)

// CreateMsg asks the shell to create a store of Kind for Dir. Like OpenMsg it
// carries only the operator's choice; the form and the creation are the
// shell's.
type CreateMsg struct {
	Kind StoreKind
	Dir  string
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

	// createDir is the directory the picker offers to create a store for, as
	// two rows above the registry entries. Empty offers nothing.
	createDir string

	// loading is the visual loading state; inflight guards a manual reload
	// against a listing already on its way. generation identifies the listing
	// each result belongs to.
	loading    bool
	inflight   bool
	generation int

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

// Init loads the store list. Every open lists again: a store registered from
// another terminal must appear without restarting the app, so an open is never
// suppressed by a listing left in flight from an earlier one.
func (m *Model) Init() tea.Cmd {
	return m.startListing()
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
// Row movement, open and reload read the board keybinding context: the picker
// is a single scrolling list of rows, the same shape a board column is, and a
// context of its own would ask the operator to rebind the same movement twice.
func (m *Model) HandleKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch {
	case m.keys.Match(config.BoardContext, config.BoardActionOpenDetail, msg):
		if kinds := m.createKinds(); m.selectedRow < len(kinds) {
			create := CreateMsg{Kind: kinds[m.selectedRow], Dir: m.createDir}
			return true, func() tea.Msg { return create }
		}
		entry, ok := m.SelectedEntry()
		if !ok {
			return true, nil
		}
		return true, func() tea.Msg { return OpenMsg{Entry: entry} }
	case m.keys.Match(config.BoardContext, config.BoardActionMoveUp, msg):
		m.moveRow(-1)
		return true, nil
	case m.keys.Match(config.BoardContext, config.BoardActionMoveDown, msg):
		m.moveRow(1)
		return true, nil
	case m.keys.Match(config.BoardContext, config.BoardActionReload, msg):
		return true, m.reload()
	}
	return false, nil
}

// View renders the picker full screen.
func (m *Model) View(spinnerFrame int, help string) string {
	errText := ""
	if m.err != nil {
		errText = m.err.Error()
	}

	rows := make([]uistorepicker.Row, 0, m.rowCount())
	for _, kind := range m.createKinds() {
		label := "Create a local store in " + m.createDir
		if kind == CentralStore {
			label = "Create a central store for " + m.createDir
		}
		rows = append(rows, uistorepicker.Row{Action: label})
	}
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

// SetCreateTarget offers to create a store for dir, or withdraws the offer
// when dir is empty.
func (m *Model) SetCreateTarget(dir string) {
	m.createDir = dir
	m.clampSelection()
}

// createKinds is what the create rows offer, in the order they are drawn.
func (m *Model) createKinds() []StoreKind {
	if m.createDir == "" {
		return nil
	}
	return []StoreKind{LocalStore, CentralStore}
}

// rowCount is every selectable row: the create rows, then the stores.
func (m *Model) rowCount() int {
	return len(m.createKinds()) + len(m.entries)
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

// SelectedEntry returns the highlighted store, or false when the highlight is
// on a create row or the list is empty.
func (m *Model) SelectedEntry() (storecatalog.Entry, bool) {
	row := m.selectedRow - len(m.createKinds())
	if row < 0 || row >= len(m.entries) {
		return storecatalog.Entry{}, false
	}
	return m.entries[row], true
}

// reload answers the manual reload key. A second read while one is in flight
// would return the same registry, so it is suppressed rather than dispatched.
func (m *Model) reload() tea.Cmd {
	if m.inflight {
		m.logger.Debug("store listing re-entry suppressed; one is already in flight")
		return nil
	}
	return m.startListing()
}

func (m *Model) startListing() tea.Cmd {
	m.generation++
	m.inflight = true
	m.loading = true
	m.err = nil
	return loadStoresCmd(m.ctx, m.catalog, m.generation)
}

func (m *Model) apply(msg StoresLoadedMsg) {
	// A result from a listing this picker has moved on from carries stale
	// entries; dropping it also leaves the current listing's in-flight state
	// intact.
	if msg.Generation != m.generation {
		return
	}

	m.loading = false
	m.inflight = false
	m.err = msg.Err

	if msg.Err != nil {
		m.logger.Error("failed to list central task-manager stores", "error", msg.Err)
		// Keep the stale rows on screen; the inline error row says why they may
		// be out of date. The clamp below still runs: the error row costs one
		// store row, so the scroll window narrows even though the rows did not
		// change.
	} else {
		m.entries = msg.Entries
	}

	m.clampSelection()
}

func (m *Model) clampSelection() {
	total := m.rowCount()
	if total == 0 {
		m.selectedRow = 0
		m.scrollOffset = 0
		return
	}
	if m.selectedRow < 0 {
		m.selectedRow = 0
	}
	if m.selectedRow >= total {
		m.selectedRow = total - 1
	}
	capacity := m.itemCapacity()
	if maxOffset := total - capacity; m.scrollOffset > maxOffset {
		m.scrollOffset = max(maxOffset, 0)
	}
	m.scrollOffset = scroll.EnsureVisible(m.scrollOffset, m.selectedRow, capacity)
}

func (m *Model) moveRow(delta int) {
	if m.rowCount() == 0 {
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

func loadStoresCmd(ctx context.Context, catalog storecatalog.Catalog, generation int) tea.Cmd {
	return func() tea.Msg {
		// A programmatic embed can build the shell without a catalog. Report
		// that on the picker rather than panicking on the first listing.
		if catalog == nil {
			return StoresLoadedMsg{Err: errors.New("no store catalog is configured for this session"), Generation: generation}
		}
		entries, err := catalog.Stores(ctx)
		return StoresLoadedMsg{Entries: entries, Err: err, Generation: generation}
	}
}
