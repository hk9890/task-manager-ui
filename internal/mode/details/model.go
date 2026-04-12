package details

import (
	"reflect"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/domain"
	uidetails "github.com/hk9890/beads-workbench/internal/ui/details"
)

const defaultLineCountWidth = 80

// Model is the shell-owned standalone detail presentation state.
type Model struct {
	SelectionID           string
	TargetID              string
	Detail                domain.IssueDetail
	Loading               bool
	Error                 string
	Keys                  config.ResolvedKeyBindings
	FocusPane             uidetails.FocusPane
	MetadataSelectedField uidetails.MetadataFieldKey

	BrowserGroupParentID string
	BrowserItems         []domain.IssueReference
	BrowserSelectedIndex int

	ScrollOffset int

	cachedLineCount      int
	cachedLineCountWidth int
	cachedViewportHeight int
	cachedSelectionID    string
	cachedTargetID       string
	cachedDetail         domain.IssueDetail
	cachedBrowserItems   []domain.IssueReference
	cachedBrowserIssueID string
	cachedLoading        bool
	cachedError          string
	cachedMetadataField  uidetails.MetadataFieldKey

	pendingOpenStatusDialog bool
}

// OpenRelatedIssueIntent requests shell-level navigation to another issue from
// dedicated detail mode.
type OpenRelatedIssueIntent struct {
	IssueID string
}

// ApplyLoadedDetail stores loaded detail and updates browser-panel state.
func (m *Model) ApplyLoadedDetail(issueID string, detail domain.IssueDetail) {
	m.Detail = detail
	m.syncBrowserPanel(issueID)
}

// SelectBrowserIssue updates the highlighted browser item for a target issue.
func (m *Model) SelectBrowserIssue(issueID string) {
	m.selectBrowserIssue(issueID)
}

// View renders the detail surface for pane and dedicated detail mode.
func (m *Model) View(maxWidth, viewportHeight int, compact bool) string {
	content := uidetails.Render(uidetails.State{
		SelectionID: m.SelectionID,
		TargetID:    m.TargetID,
		Detail:      m.Detail,
		BrowserItems: func() []domain.IssueReference {
			return append([]domain.IssueReference(nil), m.BrowserItems...)
		}(),
		BrowserSelectedIssueID: m.browserSelectedIssueID(),
		Loading:                m.Loading,
		Error:                  m.Error,
		Width:                  maxWidth,
		Compact:                compact,
		FocusPane:              m.focusPane(),
		MetadataSelectedField:  m.metadataSelectedField(),
	})

	if compact || viewportHeight <= 0 {
		if compact {
			m.invalidateLineCountCache()
		}
		return content
	}

	lines := strings.Split(content, "\n")
	m.setLineCountCache(len(lines), maxWidth, viewportHeight)
	maxOffset := maxScrollOffset(len(lines), viewportHeight)
	offset := m.ScrollOffset
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}

	start := offset
	end := start + viewportHeight
	if end > len(lines) {
		end = len(lines)
	}

	return strings.Join(lines[start:end], "\n")
}

// ClampScroll keeps scroll offset inside current content bounds.
func (m *Model) ClampScroll(maxWidth, viewportHeight int) {
	if m.cachedViewportHeight != 0 && m.cachedViewportHeight != viewportHeight {
		m.invalidateLineCountCache()
	}
	total := m.lineCountForScroll(maxWidth)
	maxOffset := maxScrollOffset(total, viewportHeight)
	if m.ScrollOffset < 0 {
		m.ScrollOffset = 0
	}
	if m.ScrollOffset > maxOffset {
		m.ScrollOffset = maxOffset
	}
}

// HandleKey updates detail-mode scroll state and reports whether it consumed the key.
func (m *Model) HandleKey(msg tea.KeyMsg, maxWidth, viewportHeight int) (bool, *OpenRelatedIssueIntent) {
	if viewportHeight <= 0 {
		return false, nil
	}
	if m.cachedViewportHeight != 0 && m.cachedViewportHeight != viewportHeight {
		m.invalidateLineCountCache()
	}
	m.normalizeRelatedSelection()
	m.ensureMetadataSelection()
	if !m.hasBrowserPanel() && m.focusPane() == uidetails.FocusPaneBrowser {
		m.FocusPane = uidetails.FocusPaneContent
	}
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

	if msg.Type == tea.KeyEnter && m.focusPane() == uidetails.FocusPaneBrowser {
		if ref, ok := m.selectedRelatedIssue(); ok {
			return true, &OpenRelatedIssueIntent{IssueID: ref.ID}
		}
		return true, nil
	}

	if msg.Type == tea.KeyEnter && m.focusPane() == uidetails.FocusPaneMetadata {
		if m.metadataSelectedField() == uidetails.MetadataFieldStatus {
			m.pendingOpenStatusDialog = true
		}
		return true, nil
	}

	total := m.lineCountForScroll(maxWidth)
	maxOffset := maxScrollOffset(total, viewportHeight)

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
		if m.focusPane() != uidetails.FocusPaneContent {
			return true, nil
		}
		m.ScrollOffset = 0
		return true, nil
	case m.Keys.Match(config.DetailContext, config.DetailActionEnd, msg):
		action = config.DetailActionEnd
		if m.focusPane() != uidetails.FocusPaneContent {
			return true, nil
		}
		m.ScrollOffset = maxOffset
		return true, nil
	default:
		return false, nil
	}

	if m.focusPane() == uidetails.FocusPaneBrowser {
		switch action {
		case config.DetailActionScrollUp:
			m.moveRelatedSelection(-1)
			return true, nil
		case config.DetailActionScrollDown:
			m.moveRelatedSelection(1)
			return true, nil
		default:
			return true, nil
		}
	}

	if m.focusPane() == uidetails.FocusPaneMetadata {
		return true, nil
	}

	if move == 0 {
		return false, nil
	}

	next := m.ScrollOffset + move
	if next < 0 {
		next = 0
	}
	if next > maxOffset {
		next = maxOffset
	}
	m.ScrollOffset = next
	return true, nil
}

func (m *Model) focusPane() uidetails.FocusPane {
	switch m.FocusPane {
	case uidetails.FocusPaneBrowser, uidetails.FocusPaneContent, uidetails.FocusPaneMetadata:
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
		if m.hasBrowserPanel() {
			m.FocusPane = uidetails.FocusPaneBrowser
		}
	}
}

func (m *Model) moveFocusRight() {
	switch m.focusPane() {
	case uidetails.FocusPaneBrowser:
		m.FocusPane = uidetails.FocusPaneContent
	case uidetails.FocusPaneContent:
		m.FocusPane = uidetails.FocusPaneMetadata
		m.ensureMetadataSelection()
	}
}

func (m *Model) metadataSelectedField() uidetails.MetadataFieldKey {
	if m.MetadataSelectedField == uidetails.MetadataFieldNone {
		return uidetails.MetadataFieldStatus
	}
	return m.MetadataSelectedField
}

func (m *Model) ensureMetadataSelection() {
	if m.MetadataSelectedField == uidetails.MetadataFieldNone {
		m.MetadataSelectedField = uidetails.MetadataFieldStatus
	}
}

// ConsumeOpenStatusDialogIntent reports and clears pending status-dialog intent.
func (m *Model) ConsumeOpenStatusDialogIntent() bool {
	if !m.pendingOpenStatusDialog {
		return false
	}
	m.pendingOpenStatusDialog = false
	return true
}

func (m *Model) moveRelatedSelection(delta int) {
	refs := m.browserIssues()
	if len(refs) == 0 {
		m.BrowserSelectedIndex = -1
		return
	}
	m.normalizeRelatedSelection()
	next := m.BrowserSelectedIndex + delta
	if next < 0 {
		next = 0
	}
	if next >= len(refs) {
		next = len(refs) - 1
	}
	m.BrowserSelectedIndex = next
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

func (m *Model) hasBrowserPanel() bool {
	return len(m.BrowserItems) > 0
}

func (m *Model) browserSelectedIssueID() string {
	if ref, ok := m.selectedRelatedIssue(); ok {
		return ref.ID
	}
	return ""
}

func (m *Model) syncBrowserPanel(issueID string) {
	parentID := strings.TrimSpace(m.Detail.ParentGroupBrowser.Parent.ID)
	if parentID == "" {
		m.clearBrowserPanel()
		return
	}

	if m.BrowserGroupParentID != parentID || len(m.BrowserItems) == 0 {
		m.BrowserGroupParentID = parentID
		m.BrowserItems = browserItemsFromParentGroup(m.Detail.ParentGroupBrowser)
	}

	m.selectBrowserIssue(issueID)
}

func (m *Model) clearBrowserPanel() {
	m.BrowserGroupParentID = ""
	m.BrowserItems = nil
	m.BrowserSelectedIndex = -1
	if m.FocusPane == uidetails.FocusPaneBrowser {
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

func maxScrollOffset(totalLines, viewportHeight int) int {
	if viewportHeight <= 0 || totalLines <= viewportHeight {
		return 0
	}
	return totalLines - viewportHeight
}

func (m *Model) lineCountForScroll(width int) int {
	if m.cachedLineCount > 0 && m.cacheMatchesCurrentState(width) {
		return m.cachedLineCount
	}

	if width <= 0 {
		width = defaultLineCountWidth
	}

	content := uidetails.Render(uidetails.State{
		SelectionID: m.SelectionID,
		TargetID:    m.TargetID,
		Detail:      m.Detail,
		BrowserItems: func() []domain.IssueReference {
			return append([]domain.IssueReference(nil), m.BrowserItems...)
		}(),
		BrowserSelectedIssueID: m.browserSelectedIssueID(),
		Loading:                m.Loading,
		Error:                  m.Error,
		Width:                  width,
		Compact:                false,
		MetadataSelectedField:  m.metadataSelectedField(),
	})
	total := len(strings.Split(content, "\n"))
	m.setLineCountCache(total, width, m.cachedViewportHeight)
	return total
}

func (m *Model) cacheMatchesCurrentState(width int) bool {
	if width <= 0 {
		width = defaultLineCountWidth
	}

	return m.SelectionID == m.cachedSelectionID &&
		m.TargetID == m.cachedTargetID &&
		m.Loading == m.cachedLoading &&
		m.Error == m.cachedError &&
		m.metadataSelectedField() == m.cachedMetadataField &&
		reflect.DeepEqual(m.BrowserItems, m.cachedBrowserItems) &&
		m.browserSelectedIssueID() == m.cachedBrowserIssueID &&
		width == m.cachedLineCountWidth &&
		reflect.DeepEqual(m.Detail, m.cachedDetail)
}

func (m *Model) setLineCountCache(total, width, viewportHeight int) {
	m.cachedLineCount = total
	m.cachedLineCountWidth = width
	m.cachedViewportHeight = viewportHeight
	m.cachedSelectionID = m.SelectionID
	m.cachedTargetID = m.TargetID
	m.cachedDetail = m.Detail
	m.cachedBrowserItems = append([]domain.IssueReference(nil), m.BrowserItems...)
	m.cachedBrowserIssueID = m.browserSelectedIssueID()
	m.cachedLoading = m.Loading
	m.cachedError = m.Error
	m.cachedMetadataField = m.metadataSelectedField()
}

func (m *Model) invalidateLineCountCache() {
	m.cachedLineCount = 0
	m.cachedLineCountWidth = 0
	m.cachedViewportHeight = 0
}
