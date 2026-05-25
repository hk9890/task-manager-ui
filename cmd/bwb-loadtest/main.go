// bwb-loadtest is a CLI tool for generating seeded beads repositories
// for bwb load testing. It requires bd to be on PATH.
//
// Usage:
//
//	bwb-loadtest gen [flags]
//
// Exit codes:
//
//	0  success
//	1  generation or I/O failure
//	2  usage error (unknown flag, missing argument)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hk9890/beads-workbench/internal/testing/loadgen"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printUsage(stdout)
		return 0
	}

	switch args[0] {
	case "gen":
		return runGen(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "unknown subcommand %q\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func printUsage(w *os.File) {
	_, _ = fmt.Fprintln(w, `bwb-loadtest — generate seeded beads repositories for bwb load testing

Usage:
  bwb-loadtest gen [flags]     Generate a seeded .beads/ directory

Run 'bwb-loadtest gen --help' for gen flags.`)
}

// runGen implements the 'gen' subcommand.
func runGen(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("gen", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		open        int
		inProgress  int
		blocked     int
		closed      int
		density     float64
		commentsPer int
		seed        int64
		outDir      string
		outFile     string
	)

	fs.IntVar(&open, "open", 20, "Number of open issues")
	fs.IntVar(&inProgress, "in-progress", 0, "Number of in_progress issues")
	fs.IntVar(&blocked, "blocked", 0, "Number of blocked issues (each gets ≥1 blocker dep)")
	fs.IntVar(&closed, "closed", 0, "Number of closed issues")
	fs.Float64Var(&density, "density", 0.0, "Average dep edges per issue (0 = no extra edges)")
	fs.IntVar(&commentsPer, "comments-per-issue", 0, "Average comments per issue (0 = disabled; slow at scale)")
	fs.Int64Var(&seed, "seed", 1, "Random seed (mandatory for reproducibility)")
	fs.StringVar(&outDir, "dir", "", "Output directory for the seeded repo (required; must exist)")
	fs.StringVar(&outFile, "out", "load-test-report.json", "Path to write the manifest JSON (use - for stdout)")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "unexpected positional arguments: %v\n", fs.Args())
		return 2
	}

	if outDir == "" {
		_, _ = fmt.Fprintln(stderr, "error: --dir is required")
		fs.Usage()
		return 2
	}

	absDir, err := filepath.Abs(outDir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: resolve --dir: %v\n", err)
		return 1
	}
	if info, err := os.Stat(absDir); err != nil || !info.IsDir() {
		_, _ = fmt.Fprintf(stderr, "error: --dir %q must be an existing directory\n", absDir)
		return 1
	}

	spec := loadgen.Spec{
		Counts: map[string]int{
			"open":        open,
			"in_progress": inProgress,
			"blocked":     blocked,
			"closed":      closed,
		},
		DepDensity:  density,
		CommentsPer: commentsPer,
		Seed:        seed,
	}

	_, _ = fmt.Fprintf(stdout, "bwb-loadtest gen: generating %d issues (open=%d in_progress=%d blocked=%d closed=%d) density=%.2f seed=%d\n",
		spec.TotalIssues(), open, inProgress, blocked, closed, density, seed)

	manifest, err := loadgen.Generate(spec, absDir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: generate failed: %v\n", err)
		return 1
	}

	// Print any warnings.
	for _, w := range manifest.Warnings {
		_, _ = fmt.Fprintf(stdout, "warning: %s\n", w)
	}

	// Human-readable summary.
	_, _ = fmt.Fprintf(stdout, "\nGeneration complete:\n")
	_, _ = fmt.Fprintf(stdout, "  bd version:   %s\n", manifest.BdVersion)
	_, _ = fmt.Fprintf(stdout, "  issues path:  %s\n", manifest.IssuesPath)
	_, _ = fmt.Fprintf(stdout, "  actual edges: %d\n", manifest.ActualEdges)
	_, _ = fmt.Fprintf(stdout, "  counts:\n")
	for _, status := range []string{"open", "in_progress", "blocked", "closed"} {
		_, _ = fmt.Fprintf(stdout, "    %-12s %d\n", status, manifest.ActualCounts[status])
	}

	// Emit manifest JSON.
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")

	if outFile == "-" {
		_, _ = fmt.Fprintln(stdout, "\nManifest:")
		if err := enc.Encode(manifest); err != nil {
			_, _ = fmt.Fprintf(stderr, "error: marshal manifest: %v\n", err)
			return 1
		}
		return 0
	}

	// Write to file.
	absOut, err := filepath.Abs(outFile)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: resolve --out: %v\n", err)
		return 1
	}
	f, err := os.Create(absOut)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: create manifest file %q: %v\n", absOut, err)
		return 1
	}

	enc2 := json.NewEncoder(f)
	enc2.SetIndent("", "  ")
	encErr := enc2.Encode(manifest)
	closeErr := f.Close()
	if encErr != nil {
		_, _ = fmt.Fprintf(stderr, "error: write manifest: %v\n", encErr)
		return 1
	}
	if closeErr != nil {
		_, _ = fmt.Fprintf(stderr, "error: close manifest file: %v\n", closeErr)
		return 1
	}

	_, _ = fmt.Fprintf(stdout, "\nManifest written to %s\n", absOut)
	return 0
}
