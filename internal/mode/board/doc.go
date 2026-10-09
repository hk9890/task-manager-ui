// Package board is the board-mode controller: it owns column selection, scroll
// offsets, and dashboard query routing, and emits mode.SelectionChangedMsg for
// the shell to react to. Column layout is composed by internal/dashboard and
// drawn by internal/ui/board.
package board
