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
package mode
