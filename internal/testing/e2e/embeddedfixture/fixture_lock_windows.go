//go:build windows

package embeddedfixture

import "os"

// Windows stubs: the embeddedfixture package depends on sh, cp, and bd, none of
// which are available in the Windows CI image, so integration tests using this
// package don't run on Windows. The stubs exist solely to keep the package
// compiling under GOOS=windows so go vet ./... succeeds.

func acquireFileLock(_ *os.File) error { return nil }

func releaseFileLock(_ *os.File) error { return nil }
