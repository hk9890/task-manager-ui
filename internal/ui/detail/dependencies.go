package detail

import (
	"fmt"
	"strings"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/ui/shared/issuerow"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// relationshipGroup is shared relation rendering input used by multiple rails.
type relationshipGroup struct {
	Label string
	Refs  []domain.IssueReference
}

func renderDependenciesPaneLines(detail domain.IssueDetail, browserItems []domain.IssueReference, cursorIssueID string, width int, skeleton bool, skeletonPhase int) []string {
	return renderRelationshipGroups(dependencyGroups(detail, browserItems), cursorIssueID, width, skeleton, skeletonPhase)
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

func renderRelationshipGroups(groups []relationshipGroup, cursorIssueID string, width int, skeleton bool, skeletonPhase int) []string {
	out := make([]string, 0, 32)
	cursorIssueID = strings.TrimSpace(cursorIssueID)
	cursorMatched := false
	for _, group := range groups {
		ordered := orderedReferences(group.Refs)
		if len(out) > 0 {
			out = append(out, "")
		}
		if skeleton {
			out = append(out, styles.TruncateString(fmt.Sprintf("%s (%s)", group.Label, issuerow.SkeletonGlyph), width))
			out = append(out, issuerow.RenderCompactSkeleton(issuerow.SkeletonOpts{
				Width:  width,
				Seed:   len(out),
				Phase:  skeletonPhase,
				Styled: true,
			}))
			continue
		}
		out = append(out, styles.TruncateString(fmt.Sprintf("%s (%d)", group.Label, len(ordered)), width))
		if len(ordered) == 0 {
			out = append(out, styles.TruncateString("(none)", width))
			continue
		}
		for _, ref := range ordered {
			isCursor := !cursorMatched && cursorIssueID != "" && ref.ID == cursorIssueID
			if isCursor {
				cursorMatched = true
			}
			out = append(out, renderReferenceRow(ref, width, isCursor))
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

	groups := dependencyGroups(detail, browserItems)
	linePos := 0
	firstGroup := true
	for _, group := range groups {
		ordered := orderedReferences(group.Refs)
		if !firstGroup {
			linePos++ // empty separator line
		}
		linePos++ // group label line
		if len(ordered) == 0 {
			linePos++ // "(none)" line
			firstGroup = false
			continue
		}
		for _, ref := range ordered {
			if strings.TrimSpace(ref.ID) == targetID {
				return linePos
			}
			linePos++
		}
		firstGroup = false
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
func renderReferenceRow(ref domain.IssueReference, width int, isCursor bool) string {
	return issuerow.RenderReferenceCompact(issuerow.ReferenceRenderConfig{
		Issue:    ref,
		Selected: isCursor,
		Width:    width,
		Styled:   true,
	})
}
