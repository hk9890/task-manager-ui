package app

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hk9890/task-manager-ui/internal/launcher"
	"github.com/hk9890/task-manager-ui/internal/ui/toaster"
)

// projectRootMissing reports whether the active store's project path does not
// stat. Resolution checks the store directory, never the project path the
// registry records for it, so a store whose project was moved or deleted still
// opens and reads fine — only a launch that runs there fails.
//
// An empty root is not missing: a shell built without one has no path to
// check, and refusing every launch there would be a false alarm.
func projectRootMissing(root string) bool {
	if root == "" {
		return false
	}
	_, err := os.Stat(root)
	return err != nil
}

// launchNeedsProjectRoot reports whether action runs in the project path: its
// workdir is blank, which falls back to the project root, or is built from it.
// A launcher with an explicit workdir of its own is unaffected by a missing
// project and stays available.
func (m Model) launchNeedsProjectRoot(action string) bool {
	for _, definition := range m.services.Config.Launcher.Definitions {
		if definition.Action != action {
			continue
		}
		workDir := strings.TrimSpace(definition.WorkDir)
		return workDir == "" || strings.Contains(workDir, launcher.ProjectRootPlaceholder)
	}
	return false
}

// launchCmd starts a launcher, or refuses it with the reason when it would run
// in a project path that is gone. Failing at exec time would blame the
// launcher; the store is what is wrong.
func (m *Model) launchCmd(action string) tea.Cmd {
	issueContext, ok := m.selectedIssueContext()
	if !ok {
		return m.showToast("No selected issue for launcher", toaster.StyleWarn)
	}
	// Checked again at the press, not only when the store was bound: a project
	// restored while its store stays open must launch again without a switch,
	// and the footer follows from here on.
	m.projectRootMissing = projectRootMissing(m.services.ProjectRoot)
	if m.projectRootMissing && m.launchNeedsProjectRoot(action) {
		// The reason leads and the path trails: a toast clips at terminal
		// width, and a project path is long enough to push the reason off it.
		return m.showToast(fmt.Sprintf(
			"Launcher %q is off: project path not accessible: %s", action, m.services.ProjectRoot,
		), toaster.StyleWarn)
	}
	return launchActionCmd(m.ctx, m.services, action, issueContext)
}
