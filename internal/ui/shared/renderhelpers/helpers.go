package renderhelpers

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hk9890/task-manager-ui/internal/ui/styles"
)

// CompactIssueType returns a short issue-type marker.
func CompactIssueType(issueType string) string {
	switch NormalizeToken(issueType) {
	case "bug":
		return "B"
	case "task":
		return "T"
	case "feature":
		return "F"
	case "epic":
		return "E"
	case "chore":
		return "C"
	case "docs":
		return "D"
	case "spike":
		return "S"
	default:
		return "?"
	}
}

// CompactPriority returns a compact priority token.
func CompactPriority(priority int) string {
	if priority < 0 {
		priority = 0
	}
	return fmt.Sprintf("P%d", priority)
}

// CompactIssueState returns a compact status token.
func CompactIssueState(status string) string {
	switch NormalizeToken(status) {
	case "blocked":
		return "BLK"
	case "in_progress":
		return "IP"
	case "open":
		return "OPN"
	case "closed":
		return "CLS"
	case "ready":
		return "RDY"
	default:
		tok := NormalizeToken(status)
		if tok == "" {
			return "---"
		}
		tok = strings.ToUpper(tok)
		runes := []rune(tok)
		if len(runes) > 3 {
			return string(runes[:3])
		}
		return tok
	}
}

// CompactIssueStateNarrow returns a single-character status token for dense rows.
func CompactIssueStateNarrow(status string) string {
	switch NormalizeToken(status) {
	case "blocked":
		return "B"
	case "in_progress":
		return "I"
	case "open":
		return "O"
	case "closed":
		return "C"
	case "ready":
		return "R"
	default:
		tok := NormalizeToken(status)
		if tok == "" {
			return "-"
		}
		return strings.ToUpper(string([]rune(tok)[0]))
	}
}

// CompactIssueTypeStyled returns a styled short issue-type marker.
func CompactIssueTypeStyled(issueType string) string {
	token := CompactIssueType(issueType)
	return styles.IssueTypeStyle(issueType).Render(token)
}

// CompactPriorityStyled returns a styled compact priority token.
func CompactPriorityStyled(priority int) string {
	token := CompactPriority(priority)
	return styles.IssuePriorityStyle(priority).Render(token)
}

// CompactIssueStateStyled returns a styled compact status token.
func CompactIssueStateStyled(status string) string {
	token := CompactIssueState(status)
	return styles.IssueStatusStyle(status).Render(token)
}

// CompactIssueStateNarrowStyled returns a styled one-character status token.
func CompactIssueStateNarrowStyled(status string) string {
	token := CompactIssueStateNarrow(status)
	return styles.IssueStatusStyle(status).Render(token)
}

// CompactIssueIDMuted returns a muted compact display ID constrained by maxWidth.
func CompactIssueIDMuted(id string, maxWidth int) string {
	token := CompactIssueID(id, maxWidth)
	return styles.IssueIDMutedStyle.Render(token)
}

// CompactIssueID returns a compact display ID constrained by maxWidth.
func CompactIssueID(id string, maxWidth int) string {
	trimmed := strings.TrimSpace(id)
	if lipgloss.Width(trimmed) <= maxWidth {
		return trimmed
	}

	const repoPrefix = "task-manager-ui-"
	if strings.HasPrefix(trimmed, repoPrefix) {
		trimmed = strings.TrimPrefix(trimmed, repoPrefix)
		if lipgloss.Width(trimmed) <= maxWidth {
			return trimmed
		}
	}

	if maxWidth <= 1 {
		return styles.TruncateString(trimmed, maxWidth)
	}

	runes := []rune(trimmed)
	suffixWidth := maxWidth - 1
	if suffixWidth <= 0 || len(runes) <= suffixWidth {
		return trimmed
	}

	return "…" + string(runes[len(runes)-suffixWidth:])
}

// NormalizeToken lowercases and normalizes separators for token comparison.
func NormalizeToken(raw string) string {
	tok := strings.TrimSpace(strings.ToLower(raw))
	tok = strings.ReplaceAll(tok, "-", "_")
	tok = strings.ReplaceAll(tok, " ", "_")
	return tok
}
