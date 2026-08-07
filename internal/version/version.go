// Package version holds the build metadata symbols injected at link time by
// GoReleaser and the mise build task. The defaults below are what a plain
// `go build` produces.
package version

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)
