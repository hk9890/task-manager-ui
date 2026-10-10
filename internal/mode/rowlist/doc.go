// Package rowlist is the list half of a surface that is one board column: the
// selection, the scroll window, the move keys and the mouse the docs tab and
// the store search share. The surface owns its rows and draws them through
// ui/board. It is a package of its own because internal/mode cannot import
// ui/board: the tests of ui/board reach internal/mode through the test harness.
package rowlist
