// Package fakes provides lightweight test doubles used across taskmgr-ui's test suites.
//
// It collects hand-written fakes that are too small or too cross-cutting to live
// next to a single package's tests:
//
//   - FakeEditor (editor.go) — fake for the $EDITOR launch path
//   - FakeLauncher (launcher.go) — fake for the configurable launcher
//   - FakeProcessRunner (process_runner.go) — fake for subprocess execution
//   - a delayed/controllable dashboard repository (delayed_dashboard.go) for
//     exercising async board loading states
//
// These are deliberately simple structs with public knobs; prefer them over a
// mocking framework so test intent stays readable.
package fakes
