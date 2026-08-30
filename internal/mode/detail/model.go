// Package detail is the detail-mode controller: pane focus, scroll offsets,
// dependency-browser selection, and the metadata quick-edit intents the shell
// turns into dialogs. Rendering — and all pane geometry — is internal/ui/detail,
// a different package with the same name.
package detail

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/config"
	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/mode"
	"github.com/hk9890/task-manager-ui/internal/ui/detail"
	"github.com/hk9890/task-manager-ui/internal/ui/scroll"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
)

// Model is the shell-owned standalone detail presentation state.
type Model struct {
	// selectionID, targetID, loading and errText are the load protocol. They
	// are written together, in order, and only by BeginLoad and FinishLoad —
	// unexported so an incomplete fifth copy of that sequence fails to compile
	// rather than compiling and misbehaving. Read them through SelectionID(),
	// TargetID(), IsLoading() and Error().
	selectionID string
	targetID    string
	loading     bool
	errText     string

	Detail                domain.IssueDetail
	PreviewDetail         domain.IssueDetail
	Keys                  config.ResolvedKeyBindings
	FocusPane             detail.FocusPane
	MetadataSelectedField detail.MetadataFieldKey

	BrowserGroupParentID string
	BrowserItems         []domain.IssueReference
	BrowserSelectedIndex int

	ContentScrollOffset      int
	DependenciesScrollOffset int
	MetadataScrollOffset     int

	// drillDepsFocusCalls counts remaining ApplyLoadedDetail calls that belong to a
	// drill-from-Dependencies sequence. Set to 2 (placeholder + real data) by
	// SetDrillFromDepsFocus when the user presses Enter on the Dependencies pane.
	// clearBrowserPanel does not flip focus while this counter is > 0.
	// The focus decision (Dependencies if rail non-empty, Content if empty) is applied
	// when the counter reaches 0 (the real data load). Reset to 0 by ClearDrillFocus.
	drillDepsFocusCalls int
}

// OpenRelatedIssueIntent requests shell-level navigation to another issue from
// dedicated detail mode. Ref carries the already-known row data (title, type,
// status, priority) so the shell can paint an optimistic header immediately
// while the full detail loads.
type OpenRelatedIssueIntent struct {
	IssueID string
	Ref     domain.IssueReference
}

// BeginLoadOptions tunes one BeginLoad call.
type BeginLoadOptions struct {
	// Ref seeds an optimistic placeholder detail so the header and core
	// metadata render immediately instead of waiting for the repository. Nil
	// leaves whatever is on screen in place, which is what a refresh of the
	// issue already shown wants.
	Ref *domain.IssueReference

	// Drill marks a drill-in from the Dependencies rail. It keeps focus on that
	// rail across the placeholder and the real load, and always seeds the
	// placeholder, because the target is a different issue by definition.
	Drill bool
}

// BeginLoad starts a load of issueID and returns whether anything changed.
//
// It owns the whole protocol: the selection and target writes, the loading
// flag, the error clear, the browser-row sync, the drill-focus decision and the
// placeholder policy. The shell used to perform these five writes in the right
// order at four call sites, and the copies had already drifted.
//
// The Drill branch does not call SelectBrowserIssue: the rail it would anchor
// belongs to the issue being navigated away from. The placeholder
// ApplyLoadedDetail below rebuilds the rail from the target and anchors it.
//
// loadingStates() reads the loading flag to drive the header spinner, so
// BeginLoad must be what precedes every loadDetailCmd.
func (m *Model) BeginLoad(issueID string, opts BeginLoadOptions) {
	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		return
	}

	m.selectionID = issueID

	if opts.Drill {
		m.targetID = issueID
		m.loading = true
		m.errText = ""
		m.SetDrillFromDepsFocus()
		if opts.Ref != nil {
			m.ApplyLoadedDetail(issueID, PlaceholderDetail(issueID, *opts.Ref, true))
		}
		return
	}

	m.SelectBrowserIssue(issueID)

	// Seed the placeholder only when the target issue actually changes, so a
	// refresh of the issue already on screen keeps its content and scroll
	// offsets while the spinner runs.
	if opts.Ref != nil && strings.TrimSpace(m.Detail.Summary.ID) != issueID {
		// A browse selection change supersedes any pending drill-focus sequence.
		m.ClearDrillFocus()
		m.ApplyLoadedDetail(issueID, PlaceholderDetail(issueID, *opts.Ref, true))
	}

	m.loading = true
	m.errText = ""
	m.targetID = issueID
}

// FinishLoad closes the load BeginLoad opened. err non-nil records the failure
// and clears the stale detail; nil clears any previous error.
func (m *Model) FinishLoad(err error) {
	m.loading = false
	if err != nil {
		m.Detail = domain.IssueDetail{}
		m.errText = err.Error()
		// Clear any pending drill-focus counter so a later load is not treated
		// as the real-data leg of a drill sequence.
		m.ClearDrillFocus()
		return
	}
	m.errText = ""
}

// SelectionID is the issue the shell considers selected for detail.
func (m *Model) SelectionID() string { return m.selectionID }

// TargetID is the issue the in-flight load is for.
func (m *Model) TargetID() string { return m.targetID }

// IsLoading reports whether a detail load is in flight.
func (m *Model) IsLoading() bool { return m.loading }

// Error is the last load failure, or "" when the last load succeeded.
func (m *Model) Error() string { return m.errText }

// Reset returns the model to its zero state, dropping any in-flight load.
func (m *Model) Reset() { *m = Model{Keys: m.Keys} }

// SetDrillFromDepsFocus prepares the model for a drill-from-Dependencies navigation
// so that:
//   - the placeholder call does not flip focus away from the Dependencies rail, and
//   - the real data load sets focus to Dependencies if the rail is non-empty, or to
//     Content if the drilled issue has no dependencies.
//
// BeginLoad calls this for a drill; callers go through BeginLoad rather than here.
func (m *Model) SetDrillFromDepsFocus() {
	m.drillDepsFocusCalls = 2
}

// ClearDrillFocus cancels any pending drill-from-dependencies focus management.
// Call this when a load error prevents the real ApplyLoadedDetail from firing,
// or when a new selection supersedes the pending drill.
func (m *Model) ClearDrillFocus() {
	m.drillDepsFocusCalls = 0
}

// ApplyLoadedDetail stores loaded detail and updates browser-panel state.
// If issueID differs from the previously loaded issue (or no issue was loaded),
// all three scroll offsets are zeroed before ClampScroll runs.
//
// When a drill-from-Dependencies sequence is in progress (drillDepsFocusCalls > 0),
// the final call (real data) drives the focus decision: Dependencies if the rail is
// non-empty, Content if empty. Intermediate calls (the optimistic placeholder) do not
// flip focus.
func (m *Model) ApplyLoadedDetail(issueID string, d domain.IssueDetail) {
	previousID := strings.TrimSpace(m.Detail.Summary.ID)
	if previousID == "" || previousID != strings.TrimSpace(issueID) {
		m.ContentScrollOffset = 0
		m.MetadataScrollOffset = 0
		m.DependenciesScrollOffset = 0
	}
	m.Detail = d
	m.PreviewDetail = domain.IssueDetail{}
	m.syncBrowserPanel(issueID)
	if m.drillDepsFocusCalls > 0 {
		m.drillDepsFocusCalls--
		if m.drillDepsFocusCalls == 0 {
			// Real data arrived: set focus from actual rail content.
			if len(m.BrowserItems) > 0 {
				m.FocusPane = detail.FocusPaneDependencies
			} else {
				m.FocusPane = detail.FocusPaneContent
			}
		}
	}
}

// ApplyPreviewDetail stores loaded preview detail without mutating browser-panel state.
func (m *Model) ApplyPreviewDetail(d domain.IssueDetail) {
	m.PreviewDetail = d
}

// SelectBrowserIssue updates the highlighted browser item for a target issue.
func (m *Model) SelectBrowserIssue(issueID string) {
	m.selectBrowserIssue(issueID)
}

// AnchorSelection records issueID as the shell's selected issue and re-anchors
// the dependency rail on it, without starting a load.
//
// BeginLoad does both as part of the load protocol. The shell calls this on the
// two paths where it skips the load — the target is already loading, or is
// already loaded and not due for a refresh — because the rail would otherwise
// stay on whatever row the operator last moved the cursor to, and
// isPreviewingTarget() decides preview-vs-full rendering by comparing targetID
// against this selection id.
func (m *Model) AnchorSelection(issueID string) {
	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		return
	}
	m.selectionID = issueID
	m.selectBrowserIssue(issueID)
}

// View renders the detail surface for pane and dedicated detail mode.
func (m *Model) View(maxWidth, viewportHeight int, compact bool, skeletonPhase int) string {
	d := m.RenderDetail()
	blockingLoad := m.loading && !m.isPreviewingTarget() && strings.TrimSpace(m.Detail.Summary.ID) == ""
	// skeleton=true in two cases:
	// 1. preview path: target differs from selection and preview detail has not yet loaded.
	// 2. direct-nav path: a load is in flight and only the placeholder summary is
	//    present — no description, comments, or relations yet. Without this branch
	//    the user sees "(no description)" / "(none)" fallbacks during the in-flight
	//    window, which misrepresents loading state as empty content.
	previewSkeleton := m.isPreviewingTarget() && strings.TrimSpace(m.PreviewDetail.Summary.ID) == ""
	directNavSkeleton := m.loading && !m.isPreviewingTarget() &&
		strings.TrimSpace(m.Detail.Description) == "" &&
		len(m.Detail.Comments) == 0 &&
		len(m.Detail.BlockedBy) == 0 &&
		len(m.Detail.Blocks) == 0 &&
		len(m.Detail.Related) == 0
	skeletonContent := previewSkeleton || directNavSkeleton

	if compact || viewportHeight <= 0 {
		return detail.Render(detail.State{
			SelectionID: m.selectionID,
			TargetID:    m.targetID,
			Detail:      d,
			QuickActions: detail.QuickActionLabels{
				EditIssue:    m.Keys.DisplayLabel(config.ShellContext, config.ShellActionEditIssue),
				UpdateIssue:  m.Keys.DisplayLabel(config.ShellContext, config.ShellActionUpdateIssue),
				AddComment:   m.Keys.DisplayLabel(config.ShellContext, config.ShellActionCommentIssue),
				CloseIssue:   m.Keys.DisplayLabel(config.ShellContext, config.ShellActionCloseIssue),
				ReloadDetail: m.Keys.DisplayLabel(config.ShellContext, config.ShellActionReloadDetail),
			},
			Loading:       blockingLoad,
			Skeleton:      skeletonContent,
			SkeletonPhase: skeletonPhase,
			Error:         m.errText,
			Width:         maxWidth,
			Compact:       compact,
		})
	}

	return detail.Render(detail.State{
		SelectionID: m.selectionID,
		TargetID:    m.targetID,
		Detail:      d,
		QuickActions: detail.QuickActionLabels{
			EditIssue:    m.Keys.DisplayLabel(config.ShellContext, config.ShellActionEditIssue),
			UpdateIssue:  m.Keys.DisplayLabel(config.ShellContext, config.ShellActionUpdateIssue),
			AddComment:   m.Keys.DisplayLabel(config.ShellContext, config.ShellActionCommentIssue),
			CloseIssue:   m.Keys.DisplayLabel(config.ShellContext, config.ShellActionCloseIssue),
			ReloadDetail: m.Keys.DisplayLabel(config.ShellContext, config.ShellActionReloadDetail),
		},
		BrowserItems: func() []domain.IssueReference {
			return append([]domain.IssueReference(nil), m.BrowserItems...)
		}(),
		BrowserSelectedIssueID:   m.browserSelectedIssueID(),
		Loading:                  blockingLoad,
		Skeleton:                 skeletonContent,
		SkeletonPhase:            skeletonPhase,
		Error:                    m.errText,
		Width:                    maxWidth,
		Height:                   viewportHeight,
		Compact:                  false,
		FocusPane:                m.focusPane(),
		MetadataSelectedField:    m.metadataSelectedField(),
		ContentScrollOffset:      m.ContentScrollOffset,
		DependenciesScrollOffset: m.DependenciesScrollOffset,
		MetadataScrollOffset:     m.MetadataScrollOffset,
	})
}

// ClampScroll keeps all pane scroll offsets inside current content bounds.
func (m *Model) ClampScroll(maxWidth, viewportHeight int) {
	if viewportHeight <= 0 {
		return
	}
	bounds := m.paneGeometry(maxWidth, viewportHeight)
	m.ContentScrollOffset = textutil.Clamp(m.ContentScrollOffset, 0, bounds.Content)
	m.DependenciesScrollOffset = textutil.Clamp(m.DependenciesScrollOffset, 0, bounds.Dependencies)
	m.MetadataScrollOffset = textutil.Clamp(m.MetadataScrollOffset, 0, bounds.Metadata)
}

// HandleKey updates detail-mode scroll state and reports whether it consumed the key.
// HandleKey processes one key for detail mode. It reports whether the key was
// consumed, an optional drill-in intent, and an optional Cmd carrying a
// mode.ActionRequestMsg for a shell-owned action (the metadata quick-edit
// dialogs). The Cmd replaces a pair of flags the shell used to poll after every
// key press; see internal/mode/contracts.go.
func (m *Model) HandleKey(msg tea.KeyMsg, maxWidth, viewportHeight int) (bool, *OpenRelatedIssueIntent, tea.Cmd) {
	if viewportHeight <= 0 {
		return false, nil, nil
	}
	m.normalizeRelatedSelection()
	m.ensureMetadataSelection()
	if m.Keys.IsZero() {
		resolved, err := config.ResolveKeyBindings(config.DefaultKeyBindings())
		if err == nil {
			m.Keys = resolved
		}
	}

	switch msg.Type {
	case tea.KeyLeft:
		m.moveFocusLeft()
		return true, nil, nil
	case tea.KeyRight:
		m.moveFocusRight()
		return true, nil, nil
	}

	if msg.Type == tea.KeyEnter && m.focusPane() == detail.FocusPaneDependencies {
		// Wire Enter to open the highlighted related/child issue. This is
		// hardcoded (NOT keymap-driven) — Enter in the Dependencies pane is a
		// special case, consistent with how Enter in the Metadata pane works.
		if ref, ok := m.selectedRelatedIssue(); ok {
			return true, &OpenRelatedIssueIntent{IssueID: ref.ID, Ref: ref}, nil
		}
		return true, nil, nil
	}

	if msg.Type == tea.KeyEnter && m.focusPane() == detail.FocusPaneMetadata {
		switch m.metadataSelectedField() {
		case detail.MetadataFieldStatus:
			return true, nil, mode.RequestActionCmd(mode.Detail, mode.ActionOpenStatusDialog)
		case detail.MetadataFieldPriority:
			return true, nil, mode.RequestActionCmd(mode.Detail, mode.ActionOpenPriorityDialog)
		}
		return true, nil, nil
	}

	bounds := m.paneGeometry(maxWidth, viewportHeight)

	move := 0
	action := ""
	switch {
	case m.Keys.Match(config.DetailContext, config.DetailActionScrollUp, msg):
		action = config.DetailActionScrollUp
		move = -1
	case m.Keys.Match(config.DetailContext, config.DetailActionScrollDown, msg):
		action = config.DetailActionScrollDown
		move = 1
	case m.Keys.Match(config.DetailContext, config.DetailActionPageUp, msg):
		action = config.DetailActionPageUp
		move = -max(1, viewportHeight-1)
	case m.Keys.Match(config.DetailContext, config.DetailActionPageDown, msg):
		action = config.DetailActionPageDown
		move = max(1, viewportHeight-1)
	case m.Keys.Match(config.DetailContext, config.DetailActionHome, msg):
		action = config.DetailActionHome
	case m.Keys.Match(config.DetailContext, config.DetailActionEnd, msg):
		action = config.DetailActionEnd
	default:
		return false, nil, nil
	}

	switch m.focusPane() {
	case detail.FocusPaneDependencies:
		if action == config.DetailActionScrollUp {
			// Only move the cursor highlight; do NOT emit OpenRelatedIssueIntent.
			// The full detail reloads only when the user presses Enter (Q5).
			m.moveRelatedSelection(-1, maxWidth, viewportHeight)
			return true, nil, nil
		}
		if action == config.DetailActionScrollDown {
			// Only move the cursor highlight; do NOT emit OpenRelatedIssueIntent.
			// The full detail reloads only when the user presses Enter (Q5).
			m.moveRelatedSelection(1, maxWidth, viewportHeight)
			return true, nil, nil
		}
		m.DependenciesScrollOffset = applyScrollAction(m.DependenciesScrollOffset, bounds.Dependencies, action, move)
		return true, nil, nil
	case detail.FocusPaneMetadata:
		if action == config.DetailActionScrollUp {
			m.moveMetadataSelection(-1, maxWidth, viewportHeight)
			return true, nil, nil
		}
		if action == config.DetailActionScrollDown {
			m.moveMetadataSelection(1, maxWidth, viewportHeight)
			return true, nil, nil
		}
		m.MetadataScrollOffset = applyScrollAction(m.MetadataScrollOffset, bounds.Metadata, action, move)
		return true, nil, nil
	default:
		m.ContentScrollOffset = applyScrollAction(m.ContentScrollOffset, bounds.Content, action, move)
		return true, nil, nil
	}
}

func (m *Model) focusPane() detail.FocusPane {
	switch m.FocusPane {
	case detail.FocusPaneDependencies, detail.FocusPaneContent, detail.FocusPaneMetadata:
		return m.FocusPane
	default:
		return detail.FocusPaneContent
	}
}

func (m *Model) moveFocusLeft() {
	switch m.focusPane() {
	case detail.FocusPaneMetadata:
		m.FocusPane = detail.FocusPaneContent
	case detail.FocusPaneContent:
		m.FocusPane = detail.FocusPaneDependencies
	}
}

func (m *Model) moveFocusRight() {
	switch m.focusPane() {
	case detail.FocusPaneDependencies:
		m.FocusPane = detail.FocusPaneContent
	case detail.FocusPaneContent:
		m.FocusPane = detail.FocusPaneMetadata
		m.ensureMetadataSelection()
	}
}

func (m *Model) metadataSelectedField() detail.MetadataFieldKey {
	if !isEditableMetadataField(m.MetadataSelectedField) {
		return detail.MetadataFieldStatus
	}
	return m.MetadataSelectedField
}

func (m *Model) ensureMetadataSelection() {
	if !isEditableMetadataField(m.MetadataSelectedField) {
		m.MetadataSelectedField = detail.MetadataFieldStatus
	}
}

func (m *Model) moveMetadataSelection(delta, maxWidth, viewportHeight int) {
	fields := editableMetadataFields()
	if len(fields) == 0 {
		m.MetadataSelectedField = detail.MetadataFieldNone
		return
	}

	m.ensureMetadataSelection()
	index := 0
	for i, key := range fields {
		if key == m.MetadataSelectedField {
			index = i
			break
		}
	}

	next := index + delta
	if next < 0 {
		next = 0
	}
	if next >= len(fields) {
		next = len(fields) - 1
	}
	m.MetadataSelectedField = fields[next]

	// Keep the selected field inside the visible window.
	lineIdx := detail.MetadataFieldLineIndex(m.MetadataSelectedField, m.Detail)
	if lineIdx >= 0 && viewportHeight > 0 {
		geometry := m.paneGeometry(maxWidth, viewportHeight)
		total := geometry.Metadata + geometry.MetadataInnerHeight
		m.MetadataScrollOffset = scroll.EnsureVisibleClipped(m.MetadataScrollOffset, lineIdx, geometry.MetadataInnerHeight, total)
	}
}

func editableMetadataFields() []detail.MetadataFieldKey {
	return []detail.MetadataFieldKey{detail.MetadataFieldStatus, detail.MetadataFieldPriority}
}

func isEditableMetadataField(key detail.MetadataFieldKey) bool {
	for _, field := range editableMetadataFields() {
		if key == field {
			return true
		}
	}
	return false
}

func (m *Model) moveRelatedSelection(delta, maxWidth, viewportHeight int) bool {
	refs := m.browserIssues()
	if len(refs) == 0 {
		m.BrowserSelectedIndex = -1
		return false
	}
	m.normalizeRelatedSelection()
	previous := m.BrowserSelectedIndex
	next := m.BrowserSelectedIndex + delta
	if next < 0 {
		next = 0
	}
	if next >= len(refs) {
		next = len(refs) - 1
	}
	m.BrowserSelectedIndex = next

	// Keep the selected ref's rendered line inside the visible window.
	if viewportHeight > 0 {
		lineIdx := detail.DependencyRefLineIndex(m.BrowserSelectedIndex, m.BrowserItems, m.Detail)
		if lineIdx >= 0 {
			geometry := m.paneGeometry(maxWidth, viewportHeight)
			// Bounds are max(0, lines-inner), so lines == bound+inner whenever the
			// pane clips; when it does not, the sum is inner and nothing scrolls.
			total := geometry.Dependencies + geometry.DependenciesInnerHeight
			m.DependenciesScrollOffset = scroll.EnsureVisibleClipped(m.DependenciesScrollOffset, lineIdx, geometry.DependenciesInnerHeight, total)
		}
	}

	return m.BrowserSelectedIndex != previous
}

// RenderDetail returns detail used for content/metadata panes while keeping
// dependency-browser context anchored to the selected issue.
func (m *Model) RenderDetail() domain.IssueDetail {
	content := m.Detail
	if targetID := strings.TrimSpace(m.targetID); targetID != "" && targetID != strings.TrimSpace(m.selectionID) {
		if strings.TrimSpace(m.PreviewDetail.Summary.ID) == targetID {
			content = m.PreviewDetail
		} else {
			ref, ok := m.browserReferenceByID(targetID)
			content = PlaceholderDetail(targetID, ref, ok)
		}
	}

	content.BlockedBy = append([]domain.IssueReference(nil), m.Detail.BlockedBy...)
	content.Blocks = append([]domain.IssueReference(nil), m.Detail.Blocks...)
	content.Related = append([]domain.IssueReference(nil), m.Detail.Related...)
	content.Children = append([]domain.IssueReference(nil), m.Detail.Children...)
	content.ParentGroupBrowser = m.Detail.ParentGroupBrowser
	return content
}

func (m *Model) isPreviewingTarget() bool {
	targetID := strings.TrimSpace(m.targetID)
	if targetID == "" {
		return false
	}
	return targetID != strings.TrimSpace(m.selectionID)
}

func (m *Model) browserReferenceByID(issueID string) (domain.IssueReference, bool) {
	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		return domain.IssueReference{}, false
	}
	for _, ref := range m.BrowserItems {
		if strings.TrimSpace(ref.ID) == issueID {
			return ref, true
		}
	}
	return domain.IssueReference{}, false
}

// PlaceholderDetail returns a lightweight IssueDetail suitable for display while
// the real repository response is in-flight.  Description is empty — the caller
// must set State.Skeleton=true so the Content pane renders ▓ rows via the
// Skeleton seam (bypassing markdown rendering).
// It is exported so the app layer can call it synchronously on selection-change
// (before the repository response arrives) to reset scroll offsets immediately.
func PlaceholderDetail(issueID string, ref domain.IssueReference, ok bool) domain.IssueDetail {
	summary := domain.IssueSummary{
		ID:       strings.TrimSpace(issueID),
		Type:     ref.Type,
		Priority: ref.Priority,
		Status:   ref.Status,
	}
	if ok {
		summary.Title = ref.Title
	}

	return domain.IssueDetail{
		Summary:     summary,
		Description: "",
	}
}

func (m *Model) selectedRelatedIssue() (domain.IssueReference, bool) {
	refs := m.browserIssues()
	if len(refs) == 0 {
		return domain.IssueReference{}, false
	}
	m.normalizeRelatedSelection()
	if m.BrowserSelectedIndex < 0 || m.BrowserSelectedIndex >= len(refs) {
		return domain.IssueReference{}, false
	}
	return refs[m.BrowserSelectedIndex], true
}

func (m *Model) normalizeRelatedSelection() {
	refs := m.browserIssues()
	if len(refs) == 0 {
		m.BrowserSelectedIndex = -1
		return
	}
	if m.BrowserSelectedIndex < 0 {
		m.BrowserSelectedIndex = 0
	}
	if m.BrowserSelectedIndex >= len(refs) {
		m.BrowserSelectedIndex = len(refs) - 1
	}
}

func (m *Model) browserIssues() []domain.IssueReference {
	return m.BrowserItems
}

func (m *Model) browserSelectedIssueID() string {
	if ref, ok := m.selectedRelatedIssue(); ok {
		return ref.ID
	}
	return ""
}

func (m *Model) syncBrowserPanel(issueID string) {
	parentID := strings.TrimSpace(m.Detail.ParentGroupBrowser.Parent.ID)
	m.BrowserGroupParentID = parentID
	m.BrowserItems = browserItemsFromDependencies(m.Detail)
	if len(m.BrowserItems) == 0 {
		m.clearBrowserPanel()
		return
	}

	m.selectBrowserIssue(issueID)
}

func (m *Model) clearBrowserPanel() {
	m.BrowserGroupParentID = ""
	m.BrowserItems = nil
	m.BrowserSelectedIndex = -1
	// Do not flip focus during an in-flight drill sequence: the placeholder has no
	// dependencies yet, but the real data load will decide focus from actual rail content.
	if m.FocusPane == detail.FocusPaneDependencies && m.drillDepsFocusCalls <= 0 {
		m.FocusPane = detail.FocusPaneContent
	}
}

func (m *Model) selectBrowserIssue(issueID string) {
	if len(m.BrowserItems) == 0 {
		m.BrowserSelectedIndex = -1
		return
	}
	for i, ref := range m.BrowserItems {
		if ref.ID == issueID {
			m.BrowserSelectedIndex = i
			return
		}
	}
	m.normalizeRelatedSelection()
}

func browserItemsFromDependencies(d domain.IssueDetail) []domain.IssueReference {
	// Group order: Blocked by, Blocks, Related, Children, Parent.
	groups := [][]domain.IssueReference{
		d.BlockedBy,
		d.Blocks,
		d.Related,
		d.Children,
	}
	// The Parent group surfaces only the parent itself (the last navigable
	// row drills up to the parent). Siblings are intentionally not listed,
	// which also avoids a second `taskmgr show` per detail load.
	if strings.TrimSpace(d.ParentGroupBrowser.Parent.ID) != "" {
		groups = append(groups, []domain.IssueReference{d.ParentGroupBrowser.Parent})
	}

	// The currently-viewed issue is shown in the Content pane; it must never
	// appear in the browser panel itself.
	selfID := strings.TrimSpace(d.Summary.ID)

	seen := make(map[string]struct{}, len(d.BlockedBy)+len(d.Blocks)+len(d.Related)+len(d.Children)+1)
	out := make([]domain.IssueReference, 0, len(seen))
	for _, refs := range groups {
		ordered := append([]domain.IssueReference(nil), refs...)
		sort.SliceStable(ordered, func(i, j int) bool {
			return ordered[i].ID < ordered[j].ID
		})

		for _, ref := range ordered {
			refID := strings.TrimSpace(ref.ID)
			if refID == "" {
				continue
			}
			if refID == selfID {
				continue
			}
			if _, exists := seen[refID]; exists {
				continue
			}
			out = append(out, ref)
			seen[refID] = struct{}{}
		}
	}

	return out
}

// paneGeometry returns the renderer's scroll bounds and pane inner heights for
// the current detail at the given viewport. Every scroll calculation goes
// through here so the responsive split is computed once, by the package that
// draws the panes.
func (m *Model) paneGeometry(maxWidth, viewportHeight int) detail.ScrollOffsets {
	return detail.MaxScrollOffsets(detail.State{
		Detail:       m.Detail,
		BrowserItems: append([]domain.IssueReference(nil), m.BrowserItems...),
		Width:        maxWidth,
		Height:       viewportHeight,
	})
}

func applyScrollAction(current, maxOffset int, action string, move int) int {
	switch action {
	case config.DetailActionHome:
		return 0
	case config.DetailActionEnd:
		return maxOffset
	default:
		next := current + move
		if next < 0 {
			next = 0
		}
		if next > maxOffset {
			next = maxOffset
		}
		return next
	}
}
