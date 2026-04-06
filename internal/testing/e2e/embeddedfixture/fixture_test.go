package embeddedfixture

import "testing"

func TestPathsPointToFixtureAssets(t *testing.T) {
	t.Parallel()

	scriptPath, seedPath := Paths(t)
	if scriptPath == "" || seedPath == "" {
		t.Fatalf("expected non-empty fixture paths")
	}
}

func TestReadSeedSpecIncludesIssuesAndDependencies(t *testing.T) {
	t.Parallel()

	spec := ReadSeedSpec(t)
	if spec.Prefix == "" {
		t.Fatalf("expected prefix in seed spec")
	}

	if len(spec.Issues) == 0 {
		t.Fatalf("expected seeded issues")
	}

	if len(spec.Deps) == 0 {
		t.Fatalf("expected seeded dependencies")
	}
}
