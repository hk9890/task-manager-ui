// Package search is the search-mode controller: query draft state, result
// paging, and the async cadence around an in-flight search (a keystroke
// arriving before the previous query resolves is queued, not dropped).
// Rendering is internal/ui/search.
package search
