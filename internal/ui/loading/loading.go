// Package loading provides shared loading-feedback primitives for the app shell.
package loading

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/hk9890/beads-workbench/internal/ui/styles"
)

// Scope identifies which shell surface is loading.
type Scope string

const (
	ScopeBoard  Scope = "board"
	ScopeSearch Scope = "search"
	ScopeDetail Scope = "detail"
)

// State describes one loading state for shared rendering.
type State struct {
	Scope  Scope
	Target string
}

// View renders a user-facing loading message for a single shell surface.
func View(state State) string {
	message := "Loading…"

	switch state.Scope {
	case ScopeBoard, ScopeSearch:
		message = "Loading issues from gateway…"
	case ScopeDetail:
		if strings.TrimSpace(state.Target) != "" {
			message = fmt.Sprintf("Loading details for %s…", state.Target)
		} else {
			message = "Loading selected issue details…"
		}
	}

	return lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render("⏳ " + message)
}

// Summary renders a shared footer/status-line summary for all active loading states.
func Summary(states []State) string {
	if len(states) == 0 {
		return lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render("Idle")
	}

	scopes := make([]string, 0, len(states))
	for _, state := range states {
		scope := strings.TrimSpace(string(state.Scope))
		if scope == "" {
			continue
		}
		if slices.Contains(scopes, scope) {
			continue
		}
		scopes = append(scopes, scope)
	}

	if len(scopes) == 0 {
		return lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render("Idle")
	}

	return lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render("Loading: " + strings.Join(scopes, ", "))
}
