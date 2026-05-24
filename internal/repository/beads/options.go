package beads

import (
	"context"

	bdrunner "github.com/hk9890/beads-workbench/internal/gateway/beads"
)

// Option is a functional constructor option for [Repository].
// Options are applied in New after the Repository is initialised.
type Option func(*Repository)

// WithCommandHook installs a test-only hook that intercepts every
// command-runner call made by the lean Repository.
//
// When hook is non-nil, every call to r.run will invoke hook(ctx, req)
// instead of r.runner.Run. The hook may delegate to the real runner for
// some requests and inject an error for others. Production callers never
// pass this option; the default behaviour (hook == nil) is unchanged.
//
// Example usage (parity test scenario 10 pattern):
//
//	runner := gateway.NewCommandRunner(...)
//	repo := beads.New(runner, beads.WithCommandHook(func(ctx context.Context, req bdrunner.CommandRequest) ([]byte, error) {
//	    if shouldFail(req.Args) {
//	        return nil, fmt.Errorf("injected error")
//	    }
//	    return runner.Run(ctx, req)
//	}))
func WithCommandHook(hook func(ctx context.Context, req bdrunner.CommandRequest) ([]byte, error)) Option {
	return func(r *Repository) {
		r.hook = hook
	}
}
