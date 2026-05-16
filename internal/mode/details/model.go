package details

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
	uidetails "github.com/hk9890/beads-workbench/internal/ui/details"
	"github.com/hk9890/beads-workbench/internal/ui/loading"
)

// Model is the shell-owned standalone detail presentation state.
type Model struct {
	SelectionID           string
	TargetID              string
	Detail                domain.IssueDetail
	PreviewDetail         domain.IssueDetail
	Loading               bool
	Error                 string
	Keys                  config.ResolvedKeyBindings
	FocusPane             uidetails.FocusPane
	MetadataSelectedField uidetails.MetadataFieldKey

	BrowserGroupParentID string
	BrowserItems         []domain.IssueReference
	BrowserSelectedIndex int

	// ScrollOffset is retained as a compatibility alias to content offset.
	ScrollOffset int

	ContentScrollOffset      int
	DependenciesScrollOffset int
	MetadataScrollOffset     int

	pendingOpenStatusDialog   bool
	pendingOpenPriorityDialog bool
}

// OpenRelatedIssueIntent requests shell-level navigation to another issue from
// dedicated detail mode.
type OpenRelatedIssueIntent struct {
	IssueID string
}

// ApplyLoadedDetail stores loaded detail and updates browser-panel state.
// If issueID differs from the previously loaded issue (or no issue was loaded),
// all three scroll offsets are zeroed before ClampScroll runs.
func (m *Model) ApplyLoadedDetail(issueID string, detail domain.IssueDetail) {
	previousID := strings.TrimSpace(m.Detail.Summary.ID)
	if previousID == "" || previousID != strings.TrimSpace(issueID) {
		m.ContentScrollOffset = 0
		m.MetadataScrollOffset = 0
		m.DependenciesScrollOffset = 0
		m.ScrollOffset = 0
	}
	m.Detail = detail
	m.PreviewDetail = domain.IssueDetail{}
	m.syncBrowserPanel(issueID)
}

// ApplyPreviewDetail stores loaded preview detail without mutating browser-panel state.
func (m *Model) ApplyPreviewDetail(detail domain.IssueDetail) {
	m.PreviewDetail = detail
}

// SelectBrowserIssue updates the highlighted browser item for a target issue.
func (m *Model) SelectBrowserIssue(issueID string) {
	m.selectBrowserIssue(issueID)
}

// View renders the detail surface for pane and dedicated detail mode.
func (m *Model) View(maxWidth, viewportHeight int, compact bool) string {
	detail := m.RenderDetail()
	blockingLoad := m.Loading && !m.isPreviewingTarget()

	if compact || viewportHeight <= 0 {
		return uidetails.Render(uidetails.State{
			SelectionID: m.SelectionID,
			TargetID:    m.TargetID,
			Detail:      detail,
			QuickActions: uidetails.QuickActionLabels{
				EditIssue:    m.Keys.DisplayLabel(config.ShellContext, config.ShellActionEditIssue),
				UpdateIssue:  m.Keys.DisplayLabel(config.ShellContext, config.ShellActionUpdateIssue),
				AddComment:   m.Keys.DisplayLabel(config.ShellContext, config.ShellActionCommentIssue),
				CloseIssue:   m.Keys.DisplayLabel(config.ShellContext, config.ShellActionCloseIssue),
				ReloadDetail: m.Keys.DisplayLabel(config.ShellContext, config.ShellActionReloadDetail),
			},
			Loading: blockingLoad,
			Error:   m.Error,
			Width:   maxWidth,
			Compact: compact,
		})
	}

	m.syncLegacyScrollAlias()
	return uidetails.Render(uidetails.State{
		SelectionID: m.SelectionID,
		TargetID:    m.TargetID,
		Detail:      detail,
		QuickActions: uidetails.QuickActionLabels{
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
		Error:                    m.Error,
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
	m.syncLegacyScrollAlias()
	bounds := uidetails.MaxScrollOffsets(uidetails.State{
		Detail:       m.Detail,
		BrowserItems: append([]domain.IssueReference(nil), m.BrowserItems...),
		Width:        maxWidth,
		Height:       viewportHeight,
	})
	m.ContentScrollOffset = clampOffset(m.ContentScrollOffset, bounds.Content)
	m.DependenciesScrollOffset = clampOffset(m.DependenciesScrollOffset, bounds.Dependencies)
	m.MetadataScrollOffset = clampOffset(m.MetadataScrollOffset, bounds.Metadata)
	m.ScrollOffset = m.ContentScrollOffset
}

// HandleKey updates detail-mode scroll state and reports whether it consumed the key.
func (m *Model) HandleKey(msg tea.KeyMsg, maxWidth, viewportHeight int) (bool, *OpenRelatedIssueIntent) {
	if viewportHeight <= 0 {
		return false, nil
	}
	m.syncLegacyScrollAlias()
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
		return true, nil
	case tea.KeyRight:
		m.moveFocusRight()
		return true, nil
	}

	if msg.Type == tea.KeyEnter && m.focusPane() == uidetails.FocusPaneDependencies {
		return true, nil
	}

	if msg.Type == tea.KeyEnter && m.focusPane() == uidetails.FocusPaneMetadata {
		switch m.metadataSelectedField() {
		case uidetails.MetadataFieldStatus:
			m.pendingOpenStatusDialog = true
		case uidetails.MetadataFieldPriority:
			m.pendingOpenPriorityDialog = true
		}
		return true, nil
	}

	bounds := uidetails.MaxScrollOffsets(uidetails.State{
		Detail:       m.Detail,
		BrowserItems: append([]domain.IssueReference(nil), m.BrowserItems...),
		Width:        maxWidth,
		Height:       viewportHeight,
	})

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
		return false, nil
	}

	switch m.focusPane() {
	case uidetails.FocusPaneDependencies:
		if action == config.DetailActionScrollUp {
			if m.moveRelatedSelection(-1) {
				if ref, ok := m.selectedRelatedIssue(); ok {
					return true, &OpenRelatedIssueIntent{IssueID: ref.ID}
				}
			}
			return true, nil
		}
		if action == config.DetailActionScrollDown {
			if m.moveRelatedSelection(1) {
				if ref, ok := m.selectedRelatedIssue(); ok {
					return true, &OpenRelatedIssueIntent{IssueID: ref.ID}
				}
			}
			return true, nil
		}
		m.DependenciesScrollOffset = applyScrollAction(m.DependenciesScrollOffset, bounds.Dependencies, action, move)
		return true, nil
	case uidetails.FocusPaneMetadata:
		if action == config.DetailActionScrollUp {
			m.moveMetadataSelection(-1)
			return true, nil
		}
		if action == config.DetailActionScrollDown {
			m.moveMetadataSelection(1)
			return true, nil
		}
		m.MetadataScrollOffset = applyScrollAction(m.MetadataScrollOffset, bounds.Metadata, action, move)
		return true, nil
	default:
		m.ContentScrollOffset = applyScrollAction(m.ContentScrollOffset, bounds.Content, action, move)
		m.ScrollOffset = m.ContentScrollOffset
		return true, nil
	}
}

func (m *Model) focusPane() uidetails.FocusPane {
	switch m.FocusPane {
	case uidetails.FocusPaneDependencies, uidetails.FocusPaneContent, uidetails.FocusPaneMetadata:
		return m.FocusPane
	default:
		return uidetails.FocusPaneContent
	}
}

func (m *Model) moveFocusLeft() {
	switch m.focusPane() {
	case uidetails.FocusPaneMetadata:
		m.FocusPane = uidetails.FocusPaneContent
	case uidetails.FocusPaneContent:
		m.FocusPane = uidetails.FocusPaneDependencies
	}
}

func (m *Model) moveFocusRight() {
	switch m.focusPane() {
	case uidetails.FocusPaneDependencies:
		m.FocusPane = uidetails.FocusPaneContent
	case uidetails.FocusPaneContent:
		m.FocusPane = uidetails.FocusPaneMetadata
		m.ensureMetadataSelection()
	}
}

func (m *Model) metadataSelectedField() uidetails.MetadataFieldKey {
	if !isEditableMetadataField(m.MetadataSelectedField) {
		return uidetails.MetadataFieldStatus
	}
	return m.MetadataSelectedField
}

func (m *Model) ensureMetadataSelection() {
	if !isEditableMetadataField(m.MetadataSelectedField) {
		m.MetadataSelectedField = uidetails.MetadataFieldStatus
	}
}

func (m *Model) moveMetadataSelection(delta int) {
	fields := editableMetadataFields()
	if len(fields) == 0 {
		m.MetadataSelectedField = uidetails.MetadataFieldNone
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
}

func editableMetadataFields() []uidetails.MetadataFieldKey {
	return []uidetails.MetadataFieldKey{uidetails.MetadataFieldStatus, uidetails.MetadataFieldPriority}
}

func isEditableMetadataField(key uidetails.MetadataFieldKey) bool {
	for _, field := range editableMetadataFields() {
		if key == field {
			return true
		}
	}
	return false
}

// ConsumeOpenStatusDialogIntent reports and clears pending status-dialog intent.
func (m *Model) ConsumeOpenStatusDialogIntent() bool {
	if !m.pendingOpenStatusDialog {
		return false
	}
	m.pendingOpenStatusDialog = false
	return true
}

// ConsumeOpenPriorityDialogIntent reports and clears pending priority-dialog intent.
func (m *Model) ConsumeOpenPriorityDialogIntent() bool {
	if !m.pendingOpenPriorityDialog {
		return false
	}
	m.pendingOpenPriorityDialog = false
	return true
}

func (m *Model) moveRelatedSelection(delta int) bool {
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
	return m.BrowserSelectedIndex != previous
}

// RenderDetail returns detail used for content/metadata panes while keeping
// dependency-browser context anchored to the selected issue.
func (m *Model) RenderDetail() domain.IssueDetail {
	content := m.Detail
	if targetID := strings.TrimSpace(m.TargetID); targetID != "" && targetID != strings.TrimSpace(m.SelectionID) {
		if strings.TrimSpace(m.PreviewDetail.Summary.ID) == targetID {
			content = m.PreviewDetail
		} else {
			ref, ok := m.browserReferenceByID(targetID)
			content = loadingPreviewDetail(targetID, ref, ok)
		}
	}

	content.BlockedBy = append([]domain.IssueReference(nil), m.Detail.BlockedBy...)
	content.Blocks = append([]domain.IssueReference(nil), m.Detail.Blocks...)
	content.Related = append([]domain.IssueReference(nil), m.Detail.Related...)
	content.ParentGroupBrowser = m.Detail.ParentGroupBrowser
	return content
}

func (m *Model) isPreviewingTarget() bool {
	targetID := strings.TrimSpace(m.TargetID)
	if targetID == "" {
		return false
	}
	return targetID != strings.TrimSpace(m.SelectionID)
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

func loadingPreviewDetail(issueID string, ref domain.IssueReference, ok bool) domain.IssueDetail {
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
		Description: loading.View(loading.State{Scope: loading.ScopeDetail, Target: strings.TrimSpace(issueID)}),
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
	if m.FocusPane == uidetails.FocusPaneDependencies {
		m.FocusPane = uidetails.FocusPaneContent
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

func browserItemsFromParentGroup(group domain.ParentGroupBrowserContext) []domain.IssueReference {
	seen := make(map[string]struct{}, len(group.Children)+1)
	out := make([]domain.IssueReference, 0, len(group.Children)+1)

	if parentID := strings.TrimSpace(group.Parent.ID); parentID != "" {
		out = append(out, group.Parent)
		seen[parentID] = struct{}{}
	}

	children := append([]domain.IssueReference(nil), group.Children...)
	sort.SliceStable(children, func(i, j int) bool {
		return children[i].ID < children[j].ID
	})

	for _, child := range children {
		childID := strings.TrimSpace(child.ID)
		if childID == "" {
			continue
		}
		if _, exists := seen[childID]; exists {
			continue
		}
		out = append(out, child)
		seen[childID] = struct{}{}
	}

	return out
}

func browserItemsFromDependencies(detail domain.IssueDetail) []domain.IssueReference {
	groups := [][]domain.IssueReference{
		detail.BlockedBy,
		detail.Blocks,
		detail.Related,
	}
	if strings.TrimSpace(detail.ParentGroupBrowser.Parent.ID) != "" {
		groups = append(groups, browserItemsFromParentGroup(detail.ParentGroupBrowser))
	}

	seen := make(map[string]struct{}, len(detail.BlockedBy)+len(detail.Blocks)+len(detail.Related)+len(detail.ParentGroupBrowser.Children)+1)
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
			if _, exists := seen[refID]; exists {
				continue
			}
			out = append(out, ref)
			seen[refID] = struct{}{}
		}
	}

	return out
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

func clampOffset(value, maxOffset int) int {
	if value < 0 {
		return 0
	}
	if value > maxOffset {
		return maxOffset
	}
	return value
}

func (m *Model) syncLegacyScrollAlias() {
	if m.ScrollOffset != 0 && m.ContentScrollOffset == 0 {
		m.ContentScrollOffset = m.ScrollOffset
	}
	m.ScrollOffset = m.ContentScrollOffset
}
