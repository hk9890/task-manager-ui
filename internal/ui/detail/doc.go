// Package detail is the dedicated issue-detail renderer: the content,
// dependencies, and metadata panes of a single issue, plus the layout that
// arranges them (three-pane when wide, stacked below InspectorTwoColumnMinWidth).
//
// The boundary is deliberate — compact row and list rendering belongs to
// ui/board, ui/search, and ui/shared/issuerow, not here. Do not add list
// rendering to this package.
//
// It renders; it owns no state. The controller that drives it is
// internal/mode/detail, a different package with the same name. Pane geometry
// lives here and is returned by MaxScrollOffsets so the controller never
// re-derives the layout split.
package detail
