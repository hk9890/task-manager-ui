package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestArchitectureGuardrails(t *testing.T) {
	t.Parallel()

	modulePath := currentModulePath(t)
	pkgs := listCmdBwbPackages(t)
	firstPartyDeps := filterFirstPartyImportPaths(pkgs, modulePath)

	t.Run("no direct SQL in active product path", func(t *testing.T) {
		t.Parallel()
		assertNoForbiddenDirectImportsInFirstPartyDeps(t, pkgs, modulePath, func(dep string) bool {
			return dep == "database/sql" || dep == "database/sql/driver"
		})
	})

	t.Run("no internal/bql dependency in standalone app", func(t *testing.T) {
		t.Parallel()
		assertNoForbiddenDeps(t, firstPartyDeps, func(dep string) bool {
			return strings.Contains(dep, "/internal/bql")
		})
	})

	t.Run("no orchestration/control-plane subsystem in active product path", func(t *testing.T) {
		t.Parallel()
		orchestrationPattern := regexp.MustCompile(`(^|/)orchestration($|/)|(^|/)control[-_]?plane($|/)`)
		assertNoForbiddenDeps(t, firstPartyDeps, func(dep string) bool {
			return orchestrationPattern.MatchString(dep)
		})
	})
}

type listedPackage struct {
	ImportPath string
	Imports    []string
}

func currentModulePath(t *testing.T) string {
	t.Helper()

	moduleRoot := moduleRootDir(t)

	cmd := exec.Command("go", "list", "-m", "-f", "{{.Path}}")
	cmd.Dir = moduleRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("resolving module path failed: %v\n%s", err, output)
	}

	modulePath := strings.TrimSpace(string(output))
	if modulePath == "" {
		t.Fatal("go list -m returned empty module path")
	}

	return modulePath
}

func listCmdBwbPackages(t *testing.T) []listedPackage {
	t.Helper()

	moduleRoot := moduleRootDir(t)

	cmd := exec.Command("go", "list", "-deps", "-json", "./cmd/bwb")
	cmd.Dir = moduleRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("listing package metadata for ./cmd/bwb failed: %v\n%s", err, output)
	}

	decoder := json.NewDecoder(strings.NewReader(string(output)))
	pkgs := make([]listedPackage, 0)
	for decoder.More() {
		var pkg listedPackage
		if err := decoder.Decode(&pkg); err != nil {
			t.Fatalf("decoding go list package metadata failed: %v", err)
		}
		pkgs = append(pkgs, pkg)
	}

	if len(pkgs) == 0 {
		t.Fatal("go list returned no package metadata for ./cmd/bwb")
	}

	return pkgs
}

func moduleRootDir(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve test file path")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func filterFirstPartyImportPaths(pkgs []listedPackage, modulePath string) []string {
	deps := make([]string, 0, len(pkgs))
	for _, pkg := range pkgs {
		if strings.HasPrefix(pkg.ImportPath, modulePath+"/") {
			deps = append(deps, pkg.ImportPath)
		}
	}

	return deps
}

func assertNoForbiddenDeps(t *testing.T, deps []string, forbidden func(string) bool) {
	t.Helper()

	violations := make([]string, 0)
	for _, dep := range deps {
		if forbidden(dep) {
			violations = append(violations, dep)
		}
	}

	if len(violations) == 0 {
		return
	}

	slices.Sort(violations)
	violations = slices.Compact(violations)
	t.Fatalf("forbidden dependencies detected: %s", strings.Join(violations, ", "))
}

func assertNoForbiddenDirectImportsInFirstPartyDeps(t *testing.T, pkgs []listedPackage, modulePath string, forbidden func(string) bool) {
	t.Helper()

	violations := make([]string, 0)
	for _, pkg := range pkgs {
		if !strings.HasPrefix(pkg.ImportPath, modulePath+"/") {
			continue
		}

		for _, importedPkg := range pkg.Imports {
			if forbidden(importedPkg) {
				violations = append(violations, pkg.ImportPath+" -> "+importedPkg)
			}
		}
	}

	if len(violations) == 0 {
		return
	}

	slices.Sort(violations)
	violations = slices.Compact(violations)
	t.Fatalf("forbidden direct imports detected: %s", strings.Join(violations, ", "))
}
