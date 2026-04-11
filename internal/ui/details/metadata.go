package details

import (
	"fmt"
	"strings"

	"github.com/hk9890/beads-workbench/internal/domain"
	"github.com/hk9890/beads-workbench/internal/ui/styles"
)

type metadataField struct {
	label string
	value string
}

func renderMetadataRail(detail domain.IssueDetail, width int) []string {
	fields := metadataFields(detail)
	if len(fields) == 0 {
		return nil
	}

	labelWidth := 0
	for _, field := range fields {
		if len(field.label) > labelWidth {
			labelWidth = len(field.label)
		}
	}

	out := make([]string, 0, len(fields))
	for _, field := range fields {
		line := fmt.Sprintf("%-*s: %s", labelWidth, field.label, field.value)
		out = append(out, styles.TruncateString(line, width))
	}

	return out
}

func metadataFields(detail domain.IssueDetail) []metadataField {
	summary := detail.Summary
	fields := make([]metadataField, 0, 10)

	fields = append(fields,
		metadataField{label: "Type", value: emptyFallback(summary.Type, "(unknown)")},
		metadataField{label: "Priority", value: formatPriority(summary.Priority)},
		metadataField{label: "Status", value: emptyFallback(summary.Status, "(unknown)")},
	)

	if assignee := strings.TrimSpace(summary.Assignee); assignee != "" {
		fields = append(fields, metadataField{label: "Assignee", value: assignee})
	}

	if labels := renderLabels(summary.Labels); labels != "(none)" {
		fields = append(fields, metadataField{label: "Labels", value: labels})
	}

	if !summary.CreatedAt.IsZero() {
		fields = append(fields, metadataField{label: "Created", value: formatTime(summary.CreatedAt)})
	}

	if !summary.UpdatedAt.IsZero() {
		fields = append(fields, metadataField{label: "Updated", value: formatTime(summary.UpdatedAt)})
	}

	fields = append(fields,
		metadataField{label: "Comments", value: fmt.Sprintf("%d", len(detail.Comments))},
		metadataField{label: "Blocked by", value: fmt.Sprintf("%d", len(detail.BlockedBy))},
		metadataField{label: "Blocks", value: fmt.Sprintf("%d", len(detail.Blocks))},
		metadataField{label: "Related", value: fmt.Sprintf("%d", len(detail.Related))},
	)

	return fields
}
