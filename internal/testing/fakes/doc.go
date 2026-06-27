// Package fakes provides lightweight test doubles used across taskmgr-ui's test suites.
//
// It collects hand-written fakes that are too small or too cross-cutting to live
// next to a single package's tests:
//
//   - FakeEditor (editor.go) — fake for the $EDITOR launch path
//   - FakeLauncher (launcher.go) — fake for the configurable launcher
//   - FakeProcessRunner (process_runner.go) — fake for subprocess execution
//   - ErrorInjectingRepository (error_injecting.go) — per-method error injection
//     and call recording for failure-path tests
//   - DelayingRepository (delaying.go) — gates one repository method behind a
//     release channel for exercising async loading/in-flight states
//
// These are deliberately simple structs with public knobs; prefer them over a
// mocking framework so test intent stays readable.
package fakes
