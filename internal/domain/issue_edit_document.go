package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	issueEditMarkerEditableBegin = "<!-- BWB:EDITABLE:BEGIN -->"
	issueEditMarkerEditableEnd   = "<!-- BWB:EDITABLE:END -->"

	issueEditFieldTitleBegin       = "<!-- BWB:FIELD:TITLE:BEGIN -->"
	issueEditFieldTitleEnd         = "<!-- BWB:FIELD:TITLE:END -->"
	issueEditFieldDescriptionBegin = "<!-- BWB:FIELD:DESCRIPTION:BEGIN -->"
	issueEditFieldDescriptionEnd   = "<!-- BWB:FIELD:DESCRIPTION:END -->"
	issueEditFieldStatusBegin      = "<!-- BWB:FIELD:STATUS:BEGIN -->"
	issueEditFieldStatusEnd        = "<!-- BWB:FIELD:STATUS:END -->"
	issueEditFieldTypeBegin        = "<!-- BWB:FIELD:TYPE:BEGIN -->"
	issueEditFieldTypeEnd          = "<!-- BWB:FIELD:TYPE:END -->"
	issueEditFieldPriorityBegin    = "<!-- BWB:FIELD:PRIORITY:BEGIN -->"
	issueEditFieldPriorityEnd      = "<!-- BWB:FIELD:PRIORITY:END -->"
	issueEditFieldAssigneeBegin    = "<!-- BWB:FIELD:ASSIGNEE:BEGIN -->"
	issueEditFieldAssigneeEnd      = "<!-- BWB:FIELD:ASSIGNEE:END -->"
	issueEditFieldLabelsBegin      = "<!-- BWB:FIELD:LABELS:BEGIN -->"
	issueEditFieldLabelsEnd        = "<!-- BWB:FIELD:LABELS:END -->"
)

// IssueEditDocument captures editable fields supported by gateway update.
//
// Document contract:
//   - Editable fields: title, description, status, type, priority, assignee, labels.
//   - Read-only fields: issue id, timestamps, notes, dependencies, related issues,
//     and comments are rendered for context and ignored on parse.
//   - Round-trip mapping: parsed fields are diffed against IssueDetail and mapped
//     to UpdateIssueInput pointer semantics (including ClearLabels).
type IssueEditDocument struct {
	Title       string
	Description string
	Status      string
	Type        string
	Priority    int
	Assignee    string
	Labels      []string
}

// RenderIssueEditDocument returns a structured issue-edit markdown document.
func RenderIssueEditDocument(issue IssueDetail) string {
	labels := issueEditLabelsBlock(issue.Summary.Labels)

	return strings.TrimSpace(fmt.Sprintf(`# Beads Workbench Issue Edit

Edit values in the editable field blocks below, then save and exit your editor.
Keep section headings and marker comments unchanged.

%s
## Title
%s
%s
%s

## Description
%s
%s
%s

## Status
%s
%s
%s

## Type
%s
%s
%s

## Priority
%s
%s
%s

## Assignee
%s
%s
%s

## Labels
%s
%s
%s

%s
## Read-Only Context (ignored on save)
- ID: %s
- Created At: %s
- Updated At: %s
- Notes: %s
- Blocked By: %s
- Blocks: %s
- Related: %s
- Comments: %d
`,
		issueEditMarkerEditableBegin,
		issueEditFieldTitleBegin,
		strings.TrimSpace(issue.Summary.Title),
		issueEditFieldTitleEnd,
		issueEditFieldDescriptionBegin,
		issue.Description,
		issueEditFieldDescriptionEnd,
		issueEditFieldStatusBegin,
		strings.TrimSpace(issue.Summary.Status),
		issueEditFieldStatusEnd,
		issueEditFieldTypeBegin,
		strings.TrimSpace(issue.Summary.Type),
		issueEditFieldTypeEnd,
		issueEditFieldPriorityBegin,
		strconv.Itoa(issue.Summary.Priority),
		issueEditFieldPriorityEnd,
		issueEditFieldAssigneeBegin,
		strings.TrimSpace(issue.Summary.Assignee),
		issueEditFieldAssigneeEnd,
		issueEditFieldLabelsBegin,
		labels,
		issueEditFieldLabelsEnd,
		issueEditMarkerEditableEnd,
		issue.Summary.ID,
		issueEditFormatTime(issue.Summary.CreatedAt),
		issueEditFormatTime(issue.Summary.UpdatedAt),
		issueEditInline(issue.Notes),
		issueEditRefsInline(issue.BlockedBy),
		issueEditRefsInline(issue.Blocks),
		issueEditRefsInline(issue.Related),
		len(issue.Comments),
	)) + "\n"
}

// ParseIssueEditDocument parses the editable document content.
func ParseIssueEditDocument(content string) (IssueEditDocument, error) {
	editable, err := issueEditExtractField(content, issueEditMarkerEditableBegin, issueEditMarkerEditableEnd)
	if err != nil {
		return IssueEditDocument{}, err
	}

	title, err := issueEditExtractField(editable, issueEditFieldTitleBegin, issueEditFieldTitleEnd)
	if err != nil {
		return IssueEditDocument{}, err
	}

	description, err := issueEditExtractField(editable, issueEditFieldDescriptionBegin, issueEditFieldDescriptionEnd)
	if err != nil {
		return IssueEditDocument{}, err
	}

	status, err := issueEditExtractField(editable, issueEditFieldStatusBegin, issueEditFieldStatusEnd)
	if err != nil {
		return IssueEditDocument{}, err
	}

	issueType, err := issueEditExtractField(editable, issueEditFieldTypeBegin, issueEditFieldTypeEnd)
	if err != nil {
		return IssueEditDocument{}, err
	}

	priorityRaw, err := issueEditExtractField(editable, issueEditFieldPriorityBegin, issueEditFieldPriorityEnd)
	if err != nil {
		return IssueEditDocument{}, err
	}

	priority, err := parseIssueEditPriority(priorityRaw)
	if err != nil {
		return IssueEditDocument{}, err
	}

	assignee, err := issueEditExtractField(editable, issueEditFieldAssigneeBegin, issueEditFieldAssigneeEnd)
	if err != nil {
		return IssueEditDocument{}, err
	}

	labelsRaw, err := issueEditExtractField(editable, issueEditFieldLabelsBegin, issueEditFieldLabelsEnd)
	if err != nil {
		return IssueEditDocument{}, err
	}

	doc := IssueEditDocument{
		Title:       strings.TrimSpace(title),
		Description: description,
		Status:      strings.TrimSpace(status),
		Type:        strings.TrimSpace(issueType),
		Priority:    priority,
		Assignee:    strings.TrimSpace(assignee),
		Labels:      parseIssueEditLabels(labelsRaw),
	}

	if doc.Title == "" {
		return IssueEditDocument{}, fmt.Errorf("title is required")
	}
	if doc.Status == "" {
		return IssueEditDocument{}, fmt.Errorf("status is required")
	}
	if doc.Type == "" {
		return IssueEditDocument{}, fmt.Errorf("type is required")
	}

	return doc, nil
}

// BuildIssueUpdateInput compares parsed editable fields with original issue
// detail and returns a gateway update input.
func BuildIssueUpdateInput(original IssueDetail, edited IssueEditDocument) (UpdateIssueInput, bool) {
	input := UpdateIssueInput{}
	changed := false

	if edited.Title != original.Summary.Title {
		input.Title = ptr(edited.Title)
		changed = true
	}

	if edited.Description != original.Description {
		input.Description = ptr(edited.Description)
		changed = true
	}

	if edited.Status != original.Summary.Status {
		input.Status = ptr(edited.Status)
		changed = true
	}

	if edited.Type != original.Summary.Type {
		input.Type = ptr(edited.Type)
		changed = true
	}

	if edited.Priority != original.Summary.Priority {
		priority := edited.Priority
		input.Priority = &priority
		changed = true
	}

	if edited.Assignee != original.Summary.Assignee {
		input.Assignee = ptr(edited.Assignee)
		changed = true
	}

	if !issueEditLabelsEqual(edited.Labels, original.Summary.Labels) {
		if len(edited.Labels) == 0 {
			input.ClearLabels = true
		} else {
			input.Labels = append([]string(nil), edited.Labels...)
		}
		changed = true
	}

	return input, changed
}

func issueEditExtractField(content, beginMarker, endMarker string) (string, error) {
	start := strings.Index(content, beginMarker)
	if start < 0 {
		return "", fmt.Errorf("missing marker %q", beginMarker)
	}

	start += len(beginMarker)
	rest := content[start:]
	end := strings.Index(rest, endMarker)
	if end < 0 {
		return "", fmt.Errorf("missing marker %q", endMarker)
	}

	return strings.Trim(rest[:end], "\n"), nil
}

func parseIssueEditPriority(raw string) (int, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	if trimmed, ok := strings.CutPrefix(value, "p"); ok {
		value = trimmed
	}

	priority, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid priority %q", strings.TrimSpace(raw))
	}

	if priority < 0 || priority > 4 {
		return 0, fmt.Errorf("invalid priority %q (expected 0..4)", strings.TrimSpace(raw))
	}

	return priority, nil
}

func parseIssueEditLabels(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	labels := make([]string, 0)
	seen := make(map[string]struct{})

	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if stripped, ok := strings.CutPrefix(line, "- "); ok {
			line = strings.TrimSpace(stripped)
		}

		for _, piece := range strings.Split(line, ",") {
			label := strings.TrimSpace(piece)
			if label == "" {
				continue
			}

			if _, exists := seen[label]; exists {
				continue
			}

			seen[label] = struct{}{}
			labels = append(labels, label)
		}
	}

	return labels
}

func issueEditLabelsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func issueEditLabelsBlock(labels []string) string {
	trimmed := make([]string, 0, len(labels))
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		trimmed = append(trimmed, label)
	}

	if len(trimmed) == 0 {
		return ""
	}

	builder := strings.Builder{}
	for i, label := range trimmed {
		if i > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString("- ")
		builder.WriteString(label)
	}

	return builder.String()
}

func issueEditRefsInline(refs []IssueReference) string {
	if len(refs) == 0 {
		return "none"
	}

	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.Title == "" {
			parts = append(parts, ref.ID)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", ref.ID, ref.Title))
	}

	return strings.Join(parts, ", ")
}

func issueEditInline(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "none"
	}

	return strings.ReplaceAll(value, "\n", " ")
}

func issueEditFormatTime(ts time.Time) string {
	if ts.IsZero() {
		return "unknown"
	}

	return ts.Format(time.RFC3339)
}

func ptr[T any](value T) *T {
	return &value
}
