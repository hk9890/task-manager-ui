package memory

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
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

// knownStatus and knownType accept what either catalog offers: the SDK's own
// enum, which is what the production store validates against, plus whatever
// SeedCatalogs added.
//
// Reading DefaultCatalogs() alone made one object disagree with itself — the
// status dialog is fed by Catalogs(), so a seeded status was offered to the
// operator and then refused by the write path of the same repository. Reading
// the seeded set alone would refuse the SDK's own statuses whenever a fixture
// narrowed the catalog for a dialog test.
func knownStatus(catalogs repository.Catalogs, status string) bool {
	return slices.ContainsFunc(DefaultCatalogs().Statuses, func(o domain.StatusOption) bool { return o.Name == status }) ||
		slices.ContainsFunc(catalogs.Statuses, func(o domain.StatusOption) bool { return o.Name == status })
}

func knownType(catalogs repository.Catalogs, issueType string) bool {
	return slices.ContainsFunc(DefaultCatalogs().Types, func(o domain.TypeOption) bool { return o.Name == issueType }) ||
		slices.ContainsFunc(catalogs.Types, func(o domain.TypeOption) bool { return o.Name == issueType })
}

// statusNames and typeNames list the accepted values for an error message, the
// SDK's first and any seeded extras after, without repeating one.
func statusNames(catalogs repository.Catalogs) string {
	names := make([]string, 0, len(DefaultCatalogs().Statuses)+len(catalogs.Statuses))
	for _, option := range DefaultCatalogs().Statuses {
		names = append(names, option.Name)
	}
	for _, option := range catalogs.Statuses {
		if !slices.Contains(names, option.Name) {
			names = append(names, option.Name)
		}
	}
	return strings.Join(names, ", ")
}

func typeNames(catalogs repository.Catalogs) string {
	names := make([]string, 0, len(DefaultCatalogs().Types)+len(catalogs.Types))
	for _, option := range DefaultCatalogs().Types {
		names = append(names, option.Name)
	}
	for _, option := range catalogs.Types {
		if !slices.Contains(names, option.Name) {
			names = append(names, option.Name)
		}
	}
	return strings.Join(names, ", ")
}

func catalogNames[T any](options []T, name func(T) string) string {
	parts := make([]string, len(options))
	for i, option := range options {
		parts[i] = name(option)
	}
	return strings.Join(parts, ", ")
}

// fieldViolation is one broken constraint and the field whose inputs it reads.
// The field is what tells a write whether the violation is its own.
type fieldViolation struct {
	field string
	err   error
}

// fieldViolations lists every broken constraint, in the SDK's order so the
// first one of a multiply-invalid write matches.
func fieldViolations(operation string, catalogs repository.Catalogs, si *storedIssue) []fieldViolation {
	var out []fieldViolation
	add := func(field string, format string, args ...any) {
		out = append(out, fieldViolation{field: field, err: validationError(operation, format, args...)})
	}

	trimmedTitle := strings.TrimSpace(si.title)
	switch {
	case trimmedTitle == "":
		add("title", "must not be empty")
	case len([]rune(trimmedTitle)) > maxTitleLen:
		add("title", "must be at most %d characters after trim, got %d", maxTitleLen, len([]rune(trimmedTitle)))
	case strings.ContainsRune(si.title, '\n'):
		add("title", "must be a single line (no newline characters)")
	case hasControlChar(si.title):
		add("title", "must not contain control characters")
	}

	if !knownStatus(catalogs, si.status) {
		add("status", "unknown status %q (want one of %s)", si.status, statusNames(catalogs))
	}
	if !knownType(catalogs, si.issueType) {
		add("type", "unknown type %q (want one of %s)", si.issueType, typeNames(catalogs))
	}
	if si.priority < priorityMin || si.priority > priorityMax {
		add("priority", "must be between %d and %d, got %d", priorityMin, priorityMax, si.priority)
	}

	switch {
	case len([]rune(si.assignee)) > maxAssigneeLen:
		add("assignee", "must be at most %d characters, got %d", maxAssigneeLen, len([]rune(si.assignee)))
	case strings.ContainsRune(si.assignee, '\n'):
		add("assignee", "must be a single line (no newline characters)")
	case hasControlChar(si.assignee):
		add("assignee", "must not contain control characters")
	}

	if len(si.labels) > maxLabels {
		add("labels", "too many labels: %d (max %d)", len(si.labels), maxLabels)
	}
	for _, label := range si.labels {
		if len([]rune(label)) > maxLabelLen {
			add("labels", "label %q exceeds max length of %d", label, maxLabelLen)
		}
		if !labelRe.MatchString(label) {
			add("labels", "label %q does not match required pattern ^[a-z0-9][a-z0-9:._/-]*$", label)
		}
	}

	return out
}

// validateIssueFields refuses any broken constraint. It is what a create goes
// through: there is no stored issue for it to inherit a violation from.
func validateIssueFields(operation string, catalogs repository.Catalogs, si *storedIssue) error {
	if violations := fieldViolations(operation, catalogs, si); len(violations) > 0 {
		return violations[0].err
	}
	return nil
}

// validateIssueWrite refuses only a violation this write introduces, mirroring
// the SDK's Store.validateWrite: a write checks what it introduces, not what it
// finds.
//
// An issue can be invalid before the write — Seed, SeedFromSnapshot and a
// fixture JSONL all bypass validation, and docs/RUNNING.md invites writing one
// by hand. Refusing every later write for it froze the issue: pressing s to
// change status failed naming a label the caller never sent, with no way to fix
// it from the UI, while the production backend accepted the same write.
func validateIssueWrite(operation string, catalogs repository.Catalogs, prev, next *storedIssue) error {
	for _, violation := range fieldViolations(operation, catalogs, next) {
		if !fieldUnchanged(violation.field, prev, next) {
			return violation.err
		}
	}
	return nil
}

// fieldUnchanged reports whether prev and next carry identical inputs for the
// named constraint. A field this function does not model is reported changed,
// so a constraint added later fails closed rather than being grandfathered by
// default — the SDK's rule, kept verbatim.
func fieldUnchanged(field string, prev, next *storedIssue) bool {
	switch field {
	case "title":
		return prev.title == next.title
	case "status":
		return prev.status == next.status
	case "type":
		return prev.issueType == next.issueType
	case "priority":
		return prev.priority == next.priority
	case "assignee":
		return prev.assignee == next.assignee
	case "labels":
		return slices.Equal(prev.labels, next.labels)
	}
	return false
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
