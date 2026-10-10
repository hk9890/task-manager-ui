// Package mode contains top-level mode controllers and shell interaction
// contracts shared across board/search/detail workflows.
//
// Baseline shell keybinding conventions for v1:
//   - Mode switching: 1/2/3, b, s, tab, shift+tab
//   - Selection movement in browse modes: j/k and down/up
//   - Action entry point: enter/o opens selected issue in detail mode
//
// Selection state is routed through SelectionChangedMsg and ActionRequestMsg so
// feature modes can remain independent from shell layout details.
//
// A mode signals the shell only with a tea.Msg that a tea.Cmd delivers, never
// with a return value or a flag that the shell polls. Shared messages are in
// contracts.go; a message that only one mode emits stays in the package of
// that mode (storepicker.OpenMsg, storepicker.CreateMsg,
// detail.OpenRelatedIssueMsg).
package mode
