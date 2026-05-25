// bwb-loadtest is a CLI tool for generating seeded beads repositories
// for bwb load testing and measuring data-layer operation latencies.
// It requires bd to be on PATH.
//
// Usage:
//
//	bwb-loadtest gen [flags]
//	bwb-loadtest measure [flags]
//
// Exit codes:
//
//	0  success
//	1  generation, measurement, or I/O failure
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
	case "measure":
		return runMeasure(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "unknown subcommand %q\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func printUsage(w *os.File) {
	_, _ = fmt.Fprintln(w, `bwb-loadtest — generate and measure seeded beads repositories for bwb load testing

Usage:
  bwb-loadtest gen [flags]       Generate a seeded .beads/ directory
  bwb-loadtest measure [flags]   Measure data-layer operation latencies

Run 'bwb-loadtest gen --help' or 'bwb-loadtest measure --help' for flags.`)
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

// runMeasure implements the 'measure' subcommand.
//
// It accepts either a pre-existing .beads/ directory (via --dir) or generates
// one inline using the same flags as the 'gen' subcommand. After measurement,
// it emits:
//   - A human-readable summary table to stdout.
//   - A JSON report to --out (default: ./load-test-report.json).
func runMeasure(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("measure", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		// Input: pre-existing repo or inline generation.
		repoDir string

		// Inline generation flags (used when --dir is absent).
		open        int
		inProgress  int
		blocked     int
		closed      int
		density     float64
		commentsPer int
		seed        int64

		// Measurement flags.
		samplesCold  int
		samplesWarm  int
		issueDetailN int

		// Output.
		outFile string
	)

	fs.StringVar(&repoDir, "dir", "", "Path to an existing directory with a seeded .beads/ (if absent, generates inline)")
	fs.IntVar(&open, "open", 50, "Number of open issues (inline generation only)")
	fs.IntVar(&inProgress, "in-progress", 10, "Number of in_progress issues (inline generation only)")
	fs.IntVar(&blocked, "blocked", 5, "Number of blocked issues (inline generation only)")
	fs.IntVar(&closed, "closed", 5, "Number of closed issues (inline generation only)")
	fs.Float64Var(&density, "density", 0.5, "Average dep edges per issue (inline generation only)")
	fs.IntVar(&commentsPer, "comments-per-issue", 0, "Average comments per issue (inline generation only; slow at scale)")
	fs.Int64Var(&seed, "seed", 1, "Random seed (inline generation only)")
	fs.IntVar(&samplesCold, "samples-cold", 5, "Number of samples for cold-path operations (Dashboard cold, cache hydrate)")
	fs.IntVar(&samplesWarm, "samples-warm", 20, "Number of samples for warm-path operations (Dashboard warm, search, detail)")
	fs.IntVar(&issueDetailN, "issue-detail-n", 10, "Number of distinct issue IDs to sample for detail measurements")
	fs.StringVar(&outFile, "out", "load-test-report.json", "Path to write the JSON report (use - for stdout)")

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

	var (
		manifest *loadgen.Manifest
		measDir  string
	)

	if repoDir != "" {
		// Use pre-existing directory.
		absDir, err := filepath.Abs(repoDir)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: resolve --dir: %v\n", err)
			return 1
		}
		info, err := os.Stat(absDir)
		if err != nil || !info.IsDir() {
			_, _ = fmt.Fprintf(stderr, "error: --dir %q must be an existing directory\n", absDir)
			return 1
		}
		measDir = absDir
		_, _ = fmt.Fprintf(stdout, "bwb-loadtest measure: using existing repo at %s\n", measDir)
	} else {
		// Generate inline.
		tmpDir, err := os.MkdirTemp("", "bwb-loadtest-*")
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: create temp dir: %v\n", err)
			return 1
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()

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

		_, _ = fmt.Fprintf(stdout, "bwb-loadtest measure: generating %d issues (open=%d in_progress=%d blocked=%d closed=%d) density=%.2f seed=%d\n",
			spec.TotalIssues(), open, inProgress, blocked, closed, density, seed)

		m, err := loadgen.Generate(spec, tmpDir)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: generate failed: %v\n", err)
			return 1
		}
		manifest = m
		measDir = tmpDir

		for _, w := range manifest.Warnings {
			_, _ = fmt.Fprintf(stdout, "warning: %s\n", w)
		}
		_, _ = fmt.Fprintf(stdout, "generation complete: %d issues, %d edges\n",
			spec.TotalIssues(), manifest.ActualEdges)
	}

	// Run the measurement harness.
	_, _ = fmt.Fprintf(stdout, "bwb-loadtest measure: measuring (cold=%d warm=%d detail-n=%d)...\n",
		samplesCold, samplesWarm, issueDetailN)

	opts := loadgen.MeasureOpts{
		SamplesCold:  samplesCold,
		SamplesWarm:  samplesWarm,
		IssueDetailN: issueDetailN,
	}

	report, err := loadgen.Measure(measDir, manifest, opts)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: measure failed: %v\n", err)
		return 1
	}

	// Print human-readable summary to stdout.
	loadgen.PrintSummaryTable(report, stdout)

	// Emit JSON report.
	if outFile == "-" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			_, _ = fmt.Fprintf(stderr, "error: encode report: %v\n", err)
			return 1
		}
		return 0
	}

	absOut, err := filepath.Abs(outFile)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "error: resolve --out: %v\n", err)
		return 1
	}

	if err := loadgen.WriteReport(report, absOut); err != nil {
		_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	_, _ = fmt.Fprintf(stdout, "Report written to %s\n", absOut)
	return 0
}
