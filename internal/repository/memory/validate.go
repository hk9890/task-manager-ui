package memory

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/hk9890/task-manager-ui/internal/domain"
)

// Field constraints mirror the task-manager SDK's validateFields
// (TASK-STORAGE-SPEC §4), because this backend is what unit tests write
// through. A fixture that accepts writes the production backend rejects
// certifies flows that fail in the real app — the create dialog storing an
// uppercase label being the concrete case.
const (
	maxTitleLen    = 200
	maxAssigneeLen = 128
	maxLabelLen    = 64
	maxLabels      = 64

	priorityMin = 0
	priorityMax = 4
	// priorityDefault is tasks.PriorityDefault. An unset priority on create is
	// P2, not P0: P0 sorts to the top of every board column.
	priorityDefault = 2
)

// labelRe is the per-label pattern from §4, byte-for-byte the SDK's.
var labelRe = regexp.MustCompile(`^[a-z0-9][a-z0-9:._/\-]*$`)

// validationError builds the same shape the taskmgr backend produces for a
// tasks.ValidationError: the code plus the SDK's message text, with no field
// prefix (mapWriteErr surfaces ValidationError.Message alone).
func validationError(operation, format string, args ...any) domain.RepositoryError {
	return domain.RepositoryError{
		Code:      domain.ErrorCodeValidationFailed,
		Operation: operation,
		Message:   fmt.Sprintf(format, args...),
	}
}

// notFoundError builds the error the write path returns when the target issue
// ID resolves to nothing. It mirrors the taskmgr backend, which surfaces the
// CLI's resolve failure, so the two backends stay in step on one template
// rather than three copies that drift apart.
func notFoundError(operation, id string) domain.RepositoryError {
	return domain.RepositoryError{
		Code:      domain.ErrorCodeCommandFailed,
		Operation: operation,
		Message:   fmt.Sprintf("command exited with code 1: Error resolving %q: no issue found", id),
	}
}

func hasControlChar(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func knownStatus(status string) bool {
	for _, option := range DefaultCatalogs().Statuses {
		if option.Name == status {
			return true
		}
	}
	return false
}

func knownType(issueType string) bool {
	for _, option := range DefaultCatalogs().Types {
		if option.Name == issueType {
			return true
		}
	}
	return false
}

func catalogNames[T any](options []T, name func(T) string) string {
	parts := make([]string, len(options))
	for i, option := range options {
		parts[i] = name(option)
	}
	return strings.Join(parts, ", ")
}

// validateIssueFields checks one issue's self-contained invariants, in the
// SDK's order so the first failure of a multiply-invalid write matches.
func validateIssueFields(operation string, si *storedIssue) error {
	trimmedTitle := strings.TrimSpace(si.title)
	switch {
	case trimmedTitle == "":
		return validationError(operation, "must not be empty")
	case len([]rune(trimmedTitle)) > maxTitleLen:
		return validationError(operation, "must be at most %d characters after trim, got %d", maxTitleLen, len([]rune(trimmedTitle)))
	case strings.ContainsRune(si.title, '\n'):
		return validationError(operation, "must be a single line (no newline characters)")
	case hasControlChar(si.title):
		return validationError(operation, "must not contain control characters")
	}

	if !knownStatus(si.status) {
		return validationError(operation, "unknown status %q (want one of %s)", si.status,
			catalogNames(DefaultCatalogs().Statuses, func(o domain.StatusOption) string { return o.Name }))
	}
	if !knownType(si.issueType) {
		return validationError(operation, "unknown type %q (want one of %s)", si.issueType,
			catalogNames(DefaultCatalogs().Types, func(o domain.TypeOption) string { return o.Name }))
	}
	if si.priority < priorityMin || si.priority > priorityMax {
		return validationError(operation, "must be between %d and %d, got %d", priorityMin, priorityMax, si.priority)
	}

	switch {
	case len([]rune(si.assignee)) > maxAssigneeLen:
		return validationError(operation, "must be at most %d characters, got %d", maxAssigneeLen, len([]rune(si.assignee)))
	case strings.ContainsRune(si.assignee, '\n'):
		return validationError(operation, "must be a single line (no newline characters)")
	case hasControlChar(si.assignee):
		return validationError(operation, "must not contain control characters")
	}

	if len(si.labels) > maxLabels {
		return validationError(operation, "too many labels: %d (max %d)", len(si.labels), maxLabels)
	}
	for _, label := range si.labels {
		if len([]rune(label)) > maxLabelLen {
			return validationError(operation, "label %q exceeds max length of %d", label, maxLabelLen)
		}
		if !labelRe.MatchString(label) {
			return validationError(operation, "label %q does not match required pattern ^[a-z0-9][a-z0-9:._/-]*$", label)
		}
	}

	return nil
}

// dedupeLabels mirrors the SDK's dedupe on create: order-preserving, first
// occurrence wins.
func dedupeLabels(labels []string) []string {
	out := make([]string, 0, len(labels))
	seen := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	return out
}
