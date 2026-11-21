# 0007: Windows is not a supported platform

The file-based store of the task-manager SDK does not run on Windows: its file lock needs a unix
target, and Windows has no fsync for a directory.

Rejected: Windows in the CI matrix, with a process runner for Windows, because the tests of the
store backend cannot pass there.
