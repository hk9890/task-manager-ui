// Package docs is the docs-mode controller: a single-column browse surface over
// issues of type doc. A doc is not work — task-manager excludes it from the
// ready and blocked queues by construction (sdk/tasks Type.IsWork) — so an open
// doc never reaches a board column. This mode is where docs are browsed
// instead. Rows are drawn by internal/ui/board, the renderer the board uses.
package docs
