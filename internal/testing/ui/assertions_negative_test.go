package ui

// The failing side of the shared assertion helpers. Both are only ever driven
// on their passing side, and both are consumed across the repository, so a
// weakening of either would wave through a real defect in every caller at once
// while each caller still looked like it asserted something.
//
// stubTB, defined in harness_golden_gate_test.go, records a failure instead of
// aborting, which is what lets a test assert that a helper rejects bad input.

import (
	"strings"
	"testing"

	"github.com/hk9890/task-manager-ui/internal/mode"
)

func TestAssertActionRequestRejectsAHalfMatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		msg        mode.ActionRequestMsg
		wantMode   mode.ID
		wantAction mode.Action
		wantFail   bool
	}{
		{
			name:     "both fields match",
			msg:      mode.ActionRequestMsg{Mode: mode.Search, Action: mode.ActionOpenDetail},
			wantMode: mode.Search, wantAction: mode.ActionOpenDetail,
			wantFail: false,
		},
		{
			// The case an && would let through: the shell emitting the right
			// action from the wrong surface.
			name:     "right action, wrong mode",
			msg:      mode.ActionRequestMsg{Mode: mode.Board, Action: mode.ActionOpenDetail},
			wantMode: mode.Search, wantAction: mode.ActionOpenDetail,
			wantFail: true,
		},
		{
			name:     "right mode, wrong action",
			msg:      mode.ActionRequestMsg{Mode: mode.Search, Action: mode.ActionOpenStatusDialog},
			wantMode: mode.Search, wantAction: mode.ActionOpenDetail,
			wantFail: true,
		},
		{
			name:     "neither matches",
			msg:      mode.ActionRequestMsg{Mode: mode.Board, Action: mode.ActionOpenStatusDialog},
			wantMode: mode.Search, wantAction: mode.ActionOpenDetail,
			wantFail: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := &stubTB{}
			AssertActionRequest(stub, tc.msg, tc.wantMode, tc.wantAction)

			if got := len(stub.failures) > 0; got != tc.wantFail {
				t.Fatalf("reported failure = %v, want %v (failures: %v)", got, tc.wantFail, stub.failures)
			}
		})
	}
}

func TestAssertActionRequestRejectsAMessageOfTheWrongType(t *testing.T) {
	t.Parallel()

	stub := &stubTB{}
	AssertActionRequest(stub, "not a message", mode.Search, mode.ActionOpenDetail)

	if len(stub.failures) == 0 {
		t.Fatal("a non-ActionRequestMsg was accepted")
	}
}

// TestAssertStartupBoardLayoutSanityRejectsAnUnderSeparatedBoard pins the
// separator threshold from below. Only the passing side was driven, so the
// bound could be loosened until a whole board lane could go missing and the
// helper still reported green.
func TestAssertStartupBoardLayoutSanityRejectsAnUnderSeparatedBoard(t *testing.T) {
	t.Parallel()

	// A board that carries every required snippet, so the separator count is
	// the only thing left deciding the verdict.
	body := strings.Join(startupBoardRequiredSnippets, " ")

	cases := []struct {
		separators int
		wantFail   bool
	}{
		{0, true},
		{4, true},
		{5, false},
		{9, false},
	}

	for _, tc := range cases {
		t.Run(strings.Repeat("│", tc.separators), func(t *testing.T) {
			t.Parallel()

			stub := &stubTB{}
			AssertStartupBoardLayoutSanity(stub, body+" "+strings.Repeat("│", tc.separators))

			if got := len(stub.failures) > 0; got != tc.wantFail {
				t.Fatalf("%d separators: reported failure = %v, want %v (failures: %v)",
					tc.separators, got, tc.wantFail, stub.failures)
			}
		})
	}
}

// A board missing a required lane label must fail regardless of separators, so
// the count above cannot be the only thing the helper checks.
func TestAssertStartupBoardLayoutSanityRejectsAMissingLane(t *testing.T) {
	t.Parallel()

	stub := &stubTB{}
	AssertStartupBoardLayoutSanity(stub, strings.Repeat("│", 9))

	if len(stub.failures) == 0 {
		t.Fatal("a board with no lane labels was accepted")
	}
}
