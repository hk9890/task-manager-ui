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

// EnsureVisibleClipped returns the scroll offset that keeps sel on a row the
// viewer can actually see when the window is clipped by scroll indicators.
//
// It exists because a clipped pane spends rows on affordances rather than
// content: a window scrolled off the top replaces its first row with
// "… (N earlier)", and one with more content below replaces its last row with
// "… (N more)". EnsureVisible only guarantees sel lands somewhere in
// [offset, offset+window), so it is satisfied by exactly the two rows an
// indicator overwrites — the selection is then inside the window and invisible
// on screen, and the chevron marking it disappears.
//
// total is the full line count of the pane. When the pane fits (total <= window)
// nothing is clipped and the offset is 0. A window of 2 or fewer rows cannot
// reserve space for indicators, so it falls back to EnsureVisible rather than
// looping forever on an unsatisfiable constraint.
func EnsureVisibleClipped(offset, sel, window, total int) int {
	if window <= 0 {
		window = 1
	}
	if sel < 0 {
		sel = 0
	}
	if offset < 0 {
		offset = 0
	}
	if total <= window {
		return 0
	}
	if window <= 2 {
		return EnsureVisible(offset, sel, window)
	}

	maxOffset := total - window
	if offset > maxOffset {
		offset = maxOffset
	}

	// visible reports the first and last rows that render real content at off.
	visible := func(off int) (first, last int) {
		first, last = off, off+window-1
		if off > 0 {
			first++ // "… (N earlier)" overwrites this row
		}
		if off+window < total {
			last-- // "… (N more)" overwrites this row
		}
		return first, last
	}

	first, last := visible(offset)
	switch {
	case sel >= first && sel <= last:
		return offset
	case sel < first:
		// Slide up just far enough to clear the top indicator.
		if off := sel - 1; off > 0 {
			return off
		}
		return 0
	default:
		// Slide down just far enough to clear the bottom indicator.
		off := sel - window + 2
		if off > maxOffset {
			off = maxOffset
		}
		if off < 0 {
			off = 0
		}
		return off
	}
}
