package detail

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

type metadataField struct {
	key      MetadataFieldKey
	label    string
	value    string
	editable bool
}

// MetadataFieldKey identifies actionable fields in the metadata rail.
type MetadataFieldKey string

const (
	MetadataFieldNone     MetadataFieldKey = ""
	MetadataFieldPriority MetadataFieldKey = "priority"
	MetadataFieldStatus   MetadataFieldKey = "status"
)

type metadataGroup struct {
	title  string
	fields []metadataField
	labels []string
}

const metadataDividerRune = "-"

func renderMetadataRail(detail domain.IssueDetail, width int, selectedField MetadataFieldKey, skeleton bool) []string {
	groups := metadataGroups(detail, skeleton)
	if len(groups) == 0 || width < 1 {
		return nil
	}

	out := make([]string, 0, 24)
	for i, group := range groups {
		if i > 0 {
			out = append(out, renderMetadataPrefixedLine(strings.Repeat(metadataDividerRune, max(0, width-2)), width, false))
		}

		if group.title != "" {
			out = append(out, renderMetadataPrefixedLine(group.title, width, false))
		}

		labelWidth := 0
		for _, field := range group.fields {
			if len(field.label) > labelWidth {
				labelWidth = len(field.label)
			}
		}

		for _, field := range group.fields {
			line := fmt.Sprintf("%-*s: %s", labelWidth, field.label, field.value)
			selected := field.key != MetadataFieldNone && field.key == selectedField
			selected = selected && selectedField != MetadataFieldNone
			out = append(out, renderMetadataFieldLine(line, width, selected))
		}

		if len(group.labels) > 0 {
			for _, label := range group.labels {
				out = append(out, renderMetadataPrefixedLine("• "+label, width, false))
			}
		}
	}

	return out
}

func renderMetadataFieldLine(line string, width int, selected bool) string {
	return renderMetadataPrefixedLine(line, width, selected)
}

func renderMetadataPrefixedLine(content string, width int, selected bool) string {
	if width < 1 {
		return ""
	}

	prefixPlain, prefixStyled := styles.SelectionPrefix(selected, true)
	gutterWidth := len([]rune(prefixPlain))
	if width <= gutterWidth {
		if !selected {
			return strings.Repeat(" ", width)
		}
		if width == 1 {
			return styles.SelectionIndicatorStyle.Render("›")
		}
		return prefixStyled
	}

	contentWidth := width - gutterWidth
	return prefixStyled + styles.TruncateString(content, contentWidth)
}

func metadataFields(detail domain.IssueDetail) []metadataField {
	groups := metadataGroups(detail, false)
	fields := make([]metadataField, 0, 16)
	for _, group := range groups {
		fields = append(fields, group.fields...)
	}
	return fields
}

// skeletonCountValue is the placeholder string used in the Counts metadata
// group when skeleton is true, keeping the value column stable.
const skeletonCountValue = "░░"

func metadataGroups(detail domain.IssueDetail, skeleton bool) []metadataGroup {
	summary := detail.Summary
	groups := make([]metadataGroup, 0, 6)

	core := metadataGroup{title: "Core"}
	core.fields = append(core.fields,
		metadataField{label: "Type", value: emptyFallback(summary.Type, "(unknown)")},
		metadataField{key: MetadataFieldStatus, label: "Status", value: emptyFallback(summary.Status, "(unknown)"), editable: true},
		metadataField{key: MetadataFieldPriority, label: "Priority", value: formatPriority(summary.Priority), editable: true},
	)
	groups = append(groups, core)

	ownership := metadataGroup{title: "Ownership"}
	if assignee := strings.TrimSpace(summary.Assignee); assignee != "" {
		ownership.fields = append(ownership.fields, metadataField{label: "Assignee", value: assignee})
	}
	if creator := strings.TrimSpace(detail.Creator); creator != "" {
		ownership.fields = append(ownership.fields, metadataField{label: "Owner", value: creator})
	}
	if len(ownership.fields) > 0 {
		groups = append(groups, ownership)
	}

	timeGroup := metadataGroup{title: "Time"}
	if !summary.CreatedAt.IsZero() {
		timeGroup.fields = append(timeGroup.fields, metadataField{label: "Created", value: formatTime(summary.CreatedAt)})
	}
	if !summary.UpdatedAt.IsZero() {
		timeGroup.fields = append(timeGroup.fields, metadataField{label: "Updated", value: formatTime(summary.UpdatedAt)})
	}
	if !summary.CreatedAt.IsZero() && !detail.ClosedAt.IsZero() {
		timeGroup.fields = append(timeGroup.fields, metadataField{label: "Duration", value: formatDuration(summary.CreatedAt, detail.ClosedAt)})
	}
	if len(timeGroup.fields) > 0 {
		groups = append(groups, timeGroup)
	}

	closeGroup := metadataGroup{title: "Close"}
	if !detail.ClosedAt.IsZero() {
		closeGroup.fields = append(closeGroup.fields, metadataField{label: "Closed", value: formatTime(detail.ClosedAt)})
	}
	if reason := strings.TrimSpace(detail.CloseReason); reason != "" {
		closeGroup.fields = append(closeGroup.fields, metadataField{label: "Reason", value: reason})
	}
	if len(closeGroup.fields) > 0 {
		groups = append(groups, closeGroup)
	}

	if labels := renderLabelLines(summary.Labels); len(labels) > 0 {
		groups = append(groups, metadataGroup{title: "Labels", labels: labels})
	}

	counts := metadataGroup{title: "Counts"}
	if skeleton {
		counts.fields = append(counts.fields,
			metadataField{label: "Comments", value: skeletonCountValue},
			metadataField{label: "Blocked by", value: skeletonCountValue},
			metadataField{label: "Blocks", value: skeletonCountValue},
			metadataField{label: "Related", value: skeletonCountValue},
			metadataField{label: "Children", value: skeletonCountValue},
		)
	} else {
		counts.fields = append(counts.fields,
			metadataField{label: "Comments", value: fmt.Sprintf("%d", len(detail.Comments))},
			metadataField{label: "Blocked by", value: fmt.Sprintf("%d", len(detail.BlockedBy))},
			metadataField{label: "Blocks", value: fmt.Sprintf("%d", len(detail.Blocks))},
			metadataField{label: "Related", value: fmt.Sprintf("%d", len(detail.Related))},
			metadataField{label: "Children", value: fmt.Sprintf("%d", len(detail.Children))},
		)
	}
	groups = append(groups, counts)

	return groups
}

func renderLabelLines(labels []string) []string {
	if len(labels) == 0 {
		return nil
	}

	trimmed := make([]string, 0, len(labels))
	for _, label := range labels {
		if text := strings.TrimSpace(label); text != "" {
			trimmed = append(trimmed, text)
		}
	}
	if len(trimmed) == 0 {
		return nil
	}

	sort.Strings(trimmed)
	return trimmed
}

func formatDuration(start, end time.Time) string {
	delta := end.Sub(start)
	if delta < 0 {
		delta = -delta
	}

	totalMinutes := int(delta / time.Minute)
	days := totalMinutes / (24 * 60)
	hours := (totalMinutes % (24 * 60)) / 60
	minutes := totalMinutes % 60

	parts := make([]string, 0, 3)
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if len(parts) == 0 {
		return "0m"
	}

	return strings.Join(parts, " ")
}

func orderedReferences(refs []domain.IssueReference) []domain.IssueReference {
	ordered := append([]domain.IssueReference(nil), refs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].ID < ordered[j].ID
	})
	return ordered
}
