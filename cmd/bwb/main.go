package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hk9890/beads-workbench/internal/app"
	"github.com/hk9890/beads-workbench/internal/config"
	"github.com/hk9890/beads-workbench/internal/gateway/beads"
)

func main() {
	cfg := config.Default()
	gateway := beads.NewGateway(beads.NewCommandRunner(beads.RunnerConfig{}))
	projectRoot, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to resolve project root: %v\n", err)
		os.Exit(1)
	}

	services, err := app.NewServices(gateway, cfg, projectRoot)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to initialize services: %v\n", err)
		os.Exit(1)
	}

	program := tea.NewProgram(app.NewModel(services), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "bwb failed: %v\n", err)
		os.Exit(1)
	}
}
