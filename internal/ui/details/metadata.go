package details

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/ui/styles"
)

type metadataField struct {
	label string
	value string
}

type metadataGroup struct {
	title  string
	fields []metadataField
	labels []string
}

const metadataDividerRune = "-"

func renderMetadataRail(detail domain.IssueDetail, width int) []string {
	groups := metadataGroups(detail)
	if len(groups) == 0 || width < 1 {
		return nil
	}

	out := make([]string, 0, 24)
	for i, group := range groups {
		if i > 0 {
			out = append(out, strings.Repeat(metadataDividerRune, width))
		}

		if group.title != "" {
			out = append(out, styles.TruncateString(group.title, width))
		}

		labelWidth := 0
		for _, field := range group.fields {
			if len(field.label) > labelWidth {
				labelWidth = len(field.label)
			}
		}

		for _, field := range group.fields {
			line := fmt.Sprintf("%-*s: %s", labelWidth, field.label, field.value)
			out = append(out, styles.TruncateString(line, width))
		}

		if len(group.labels) > 0 {
			for _, label := range group.labels {
				out = append(out, styles.TruncateString("• "+label, width))
			}
		}
	}

	return out
}

func metadataFields(detail domain.IssueDetail) []metadataField {
	groups := metadataGroups(detail)
	fields := make([]metadataField, 0, 16)
	for _, group := range groups {
		fields = append(fields, group.fields...)
	}
	return fields
}

func metadataGroups(detail domain.IssueDetail) []metadataGroup {
	summary := detail.Summary
	groups := make([]metadataGroup, 0, 6)

	core := metadataGroup{title: "Core"}
	core.fields = append(core.fields,
		metadataField{label: "Type", value: emptyFallback(summary.Type, "(unknown)")},
		metadataField{label: "Priority", value: formatPriority(summary.Priority)},
		metadataField{label: "Status", value: emptyFallback(summary.Status, "(unknown)")},
	)
	groups = append(groups, core)

	ownership := metadataGroup{title: "Ownership"}
	if assignee := strings.TrimSpace(summary.Assignee); assignee != "" {
		ownership.fields = append(ownership.fields, metadataField{label: "Assignee", value: assignee})
	}
	if creator := strings.TrimSpace(detail.Creator); creator != "" {
		ownership.fields = append(ownership.fields, metadataField{label: "Creator", value: creator})
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
	counts.fields = append(counts.fields,
		metadataField{label: "Comments", value: fmt.Sprintf("%d", len(detail.Comments))},
		metadataField{label: "Blocked by", value: fmt.Sprintf("%d", len(detail.BlockedBy))},
		metadataField{label: "Blocks", value: fmt.Sprintf("%d", len(detail.Blocks))},
		metadataField{label: "Related", value: fmt.Sprintf("%d", len(detail.Related))},
	)
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
