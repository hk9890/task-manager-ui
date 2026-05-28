// Package scroll provides shared viewport-scroll helpers.
package scroll

// EnsureVisible returns the scroll offset that keeps sel inside the visible
// window of size window. It slides the window as little as possible:
//
//   - If sel is above the current offset, the window slides up to sel.
//   - If sel is below the current window bottom, the window slides down so
//     sel is the last visible item.
//   - Otherwise the offset is unchanged.
//
// A window <= 0 is treated as 1. Negative sel or offset is clamped to 0.
func EnsureVisible(offset, sel, window int) int {
	if window <= 0 {
		window = 1
	}
	if sel < 0 {
		sel = 0
	}
	if offset < 0 {
		offset = 0
	}
	if sel < offset {
		return sel
	}
	if sel >= offset+window {
		return sel - window + 1
	}
	return offset
}
