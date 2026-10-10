// Package mode contains top-level mode controllers and shell interaction
// contracts shared across board/docs/search/detail workflows.
//
// Baseline shell keybinding conventions:
//   - Mode switching: tab, shift+tab
//   - Selection movement in browse modes: the arrow, page, home and end keys
//   - Action entry point: enter opens selected issue in detail mode
//   - No action is bound to a bare printable key outside a modal: a browse
//     mode types every one into its Query, the filter over its rows
//   - The store search is opened by a shell action, not by a tab: its Query is
//     what the store is searched for
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
