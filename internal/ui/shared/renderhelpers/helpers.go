package renderhelpers

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/hk9890/beads-workbench/internal/ui/styles"
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

// CompactIssueID returns a compact display ID constrained by maxWidth.
func CompactIssueID(id string, maxWidth int) string {
	trimmed := strings.TrimSpace(id)
	if lipgloss.Width(trimmed) <= maxWidth {
		return trimmed
	}

	const repoPrefix = "beads-workbench-"
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

// MaxInt returns the greater of two ints.
func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
