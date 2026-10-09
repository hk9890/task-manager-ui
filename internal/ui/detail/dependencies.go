package detail

import (
	"fmt"
	"strings"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/textutil"
)

// relationshipGroup is shared relation rendering input used by multiple rails.
type relationshipGroup struct {
	Label string
	Refs  []domain.IssueReference
}

// refMarks names the reference rows that are marked: the cursor row the keys
// move, which takes the selection gutter and band, and the row under the
// pointer, which takes the hover band.
type refMarks struct {
	cursor string
	hover  string
}

// marks is the marked rows state asks for.
func (state State) marks() refMarks {
	marks := refMarks{cursor: state.BrowserSelectedIssueID}
	if state.Hover != nil && state.Hover.Pane == FocusPaneDependencies {
		marks.hover = state.Hover.RefID
	}
	return marks
}

func renderDependenciesPaneLines(detail domain.IssueDetail, browserItems []domain.IssueReference, cursorIssueID string, width int, skeleton bool, skeletonPhase int) []string {
	return renderRelationshipGroups(dependencyGroups(detail, browserItems), refMarks{cursor: cursorIssueID}, width, skeleton, skeletonPhase)
}

// dependencyLineRefs is the reference ID each line of the dependency pane
// holds, in the order renderRelationshipGroups draws them: "" for a label, a
// separator and a "(none)" line. It mirrors that function's structure so a
// line can be mapped to its reference without rendering.
func dependencyLineRefs(groups []relationshipGroup) []string {
	out := make([]string, 0, 32)
	for _, group := range groups {
		ordered := orderedReferences(group.Refs)
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, "")
		if len(ordered) == 0 {
			out = append(out, "")
			continue
		}
		for _, ref := range ordered {
			out = append(out, ref.ID)
		}
	}
	return out
}

func dependencyGroups(detail domain.IssueDetail, browserItems []domain.IssueReference) []relationshipGroup {
	// Group order: Blocked by, Blocks, Related, Children, Parent.
	// The Parent group reads the parent ref directly from detail; browserItems
	// is used only as a capacity hint for the dedup set below.
	groups := []relationshipGroup{
		{Label: "Blocked by", Refs: detail.BlockedBy},
		{Label: "Blocks", Refs: detail.Blocks},
		{Label: "Related", Refs: detail.Related},
		{Label: "Children", Refs: detail.Children},
	}
	if parent := detail.ParentGroupBrowser.Parent; strings.TrimSpace(parent.ID) != "" {
		groups = append(groups, relationshipGroup{Label: "Parent", Refs: []domain.IssueReference{parent}})
	}

	seen := make(map[string]struct{}, len(detail.BlockedBy)+len(detail.Blocks)+len(detail.Related)+len(detail.Children)+len(browserItems))
	out := make([]relationshipGroup, 0, len(groups))
	for _, group := range groups {
		ordered := orderedReferences(group.Refs)
		filtered := make([]domain.IssueReference, 0, len(ordered))
		for _, ref := range ordered {
			refID := strings.TrimSpace(ref.ID)
			if refID == "" {
				continue
			}
			if _, exists := seen[refID]; exists {
				continue
			}
			filtered = append(filtered, ref)
			seen[refID] = struct{}{}
		}
		out = append(out, relationshipGroup{Label: group.Label, Refs: filtered})
	}

	return out
}

func renderRelationshipGroups(groups []relationshipGroup, marks refMarks, width int, skeleton bool, skeletonPhase int) []string {
	out := make([]string, 0, 32)
	cursorIssueID := strings.TrimSpace(marks.cursor)
	cursorMatched := false
	for _, group := range groups {
		ordered := orderedReferences(group.Refs)
		if len(out) > 0 {
			out = append(out, "")
		}
		if skeleton {
			out = append(out, textutil.TruncateString(fmt.Sprintf("%s (%s)", group.Label, issuerow.SkeletonGlyph), width))
			out = append(out, issuerow.RenderCompactSkeleton(issuerow.SkeletonOpts{
				Width:  width,
				Seed:   len(out),
				Phase:  skeletonPhase,
				Styled: true,
			}))
			continue
		}
		out = append(out, textutil.TruncateString(fmt.Sprintf("%s (%d)", group.Label, len(ordered)), width))
		if len(ordered) == 0 {
			out = append(out, textutil.TruncateString("(none)", width))
			continue
		}
		for _, ref := range ordered {
			isCursor := !cursorMatched && cursorIssueID != "" && ref.ID == cursorIssueID
			if isCursor {
				cursorMatched = true
			}
			out = append(out, renderReferenceRow(ref, width, isCursor, marks.hover != "" && ref.ID == marks.hover))
		}
	}
	if len(out) == 0 {
		return []string{"(none)"}
	}
	return out
}

func countDependencyReferences(detail domain.IssueDetail) int {
	return len(detail.BlockedBy) + len(detail.Blocks) + len(detail.Related) + len(detail.Children)
}

// DependencyRefLineIndex returns the zero-based line index of the
// browserItems[refIndex] entry within the rendered dependency pane line list
// (as produced by renderDependenciesPaneLines). Returns -1 if refIndex is out
// of range or if browserItems is empty.
//
// The dependency pane renders groups separated by empty lines and headed by a
// label line. This function mirrors that structure to compute the line position
// without allocating rendered strings.
func DependencyRefLineIndex(refIndex int, browserItems []domain.IssueReference, detail domain.IssueDetail) int {
	if refIndex < 0 || len(browserItems) == 0 || refIndex >= len(browserItems) {
		return -1
	}
	targetID := strings.TrimSpace(browserItems[refIndex].ID)
	if targetID == "" {
		return -1
	}

	for line, refID := range dependencyLineRefs(dependencyGroups(detail, browserItems)) {
		if refID != "" && strings.TrimSpace(refID) == targetID {
			return line
		}
	}
	return -1
}

// renderReferenceRow renders a single dependency reference row.
//
// isCursor marks the movable selection row (↑/↓ moves it; Enter commits the load).
// It is rendered with the app-wide "› " selection prefix via issuerow Selected=true —
// byte-identical to the cursor in the board, search, and metadata panes, so the one
// marker the user moves looks the same everywhere. The currently-viewed issue is never
// in this list (it is excluded when the browser panel is assembled), so it needs no
// marker here — it lives in the Content/Metadata panes.
func renderReferenceRow(ref domain.IssueReference, width int, isCursor, isHover bool) string {
	return issuerow.RenderReferenceCompact(issuerow.ReferenceRenderConfig{
		Issue:    ref,
		Selected: isCursor,
		Hovered:  isHover,
		Width:    width,
		Styled:   true,
	})
}
