package domain

import (
	"strings"
	"testing"
)

// TestSecurityDescriptionCannotHijackLabels verifies that a description body
// containing a forged TASKMGRUI:FIELD:LABELS:BEGIN/END block does not overwrite the
// real labels field that appears later in the document.
func TestSecurityDescriptionCannotHijackLabels(t *testing.T) {
	t.Parallel()

	issue := IssueDetail{Summary: IssueSummary{
		Title:    "Hijack labels test",
		Status:   "open",
		Type:     "task",
		Priority: 2,
		Labels:   []string{"real-label"},
	}, Description: "safe description"}

	rendered := RenderIssueEditDocument(issue)

	// Inject a forged labels block into the description section by replacing the
	// description text with malicious marker content. This simulates an operator
	// pasting marker-shaped text into the description field.
	forgedDesc := issueEditFieldLabelsBegin + "\ninjected-label\n" + issueEditFieldLabelsEnd
	tampered := strings.Replace(
		rendered,
		issueEditFieldDescriptionBegin+"\nsafe description\n"+issueEditFieldDescriptionEnd,
		issueEditFieldDescriptionBegin+"\n"+forgedDesc+"\n"+issueEditFieldDescriptionEnd,
		1,
	)

	_, err := ParseIssueEditDocument(tampered)
	if err == nil {
		t.Fatalf("expected parse error when description contains forged TASKMGRUI:FIELD:LABELS markers, got nil")
	}
	if !strings.Contains(err.Error(), "TASKMGRUI:") {
		t.Fatalf("expected error to reference TASKMGRUI: token, got: %v", err)
	}
}

// TestSecurityDescriptionCannotHijackStatus verifies that a description body
// containing a forged TASKMGRUI:FIELD:STATUS:BEGIN/END block does not overwrite the
// real status field that appears later in the document.
func TestSecurityDescriptionCannotHijackStatus(t *testing.T) {
	t.Parallel()

	issue := IssueDetail{Summary: IssueSummary{
		Title:    "Hijack status test",
		Status:   "open",
		Type:     "task",
		Priority: 2,
	}, Description: "safe description"}

	rendered := RenderIssueEditDocument(issue)

	// Inject a forged status block inside the description section.
	forgedDesc := issueEditFieldStatusBegin + "\ninjected-status\n" + issueEditFieldStatusEnd
	tampered := strings.Replace(
		rendered,
		issueEditFieldDescriptionBegin+"\nsafe description\n"+issueEditFieldDescriptionEnd,
		issueEditFieldDescriptionBegin+"\n"+forgedDesc+"\n"+issueEditFieldDescriptionEnd,
		1,
	)

	_, err := ParseIssueEditDocument(tampered)
	if err == nil {
		t.Fatalf("expected parse error when description contains forged TASKMGRUI:FIELD:STATUS markers, got nil")
	}
	if !strings.Contains(err.Error(), "TASKMGRUI:") {
		t.Fatalf("expected error to reference TASKMGRUI: token, got: %v", err)
	}
}

// TestSecurityDescriptionContainingEditableEndReturnsExplicitError verifies
// that a description body containing TASKMGRUI:EDITABLE:END produces a specific error
// rather than silently truncating the editable block.
func TestSecurityDescriptionContainingEditableEndReturnsExplicitError(t *testing.T) {
	t.Parallel()

	issue := IssueDetail{Summary: IssueSummary{
		Title:    "Editable end truncation test",
		Status:   "open",
		Type:     "task",
		Priority: 2,
	}, Description: "safe description"}

	rendered := RenderIssueEditDocument(issue)

	// Inject the editable-end marker inside the description field.
	tampered := strings.Replace(
		rendered,
		issueEditFieldDescriptionBegin+"\nsafe description\n"+issueEditFieldDescriptionEnd,
		issueEditFieldDescriptionBegin+"\n"+issueEditMarkerEditableEnd+"\n"+issueEditFieldDescriptionEnd,
		1,
	)

	_, err := ParseIssueEditDocument(tampered)
	if err == nil {
		t.Fatalf("expected parse error when description contains TASKMGRUI:EDITABLE:END, got nil")
	}

	// The error must be specific — it should not be a generic "missing marker"
	// but should indicate that multiple EDITABLE:END markers were detected or
	// that the description contains forbidden TASKMGRUI: tokens.
	errMsg := err.Error()
	if !strings.Contains(errMsg, "TASKMGRUI:") && !strings.Contains(errMsg, "multiple") {
		t.Fatalf("expected error to mention TASKMGRUI: or multiple markers conflict; got: %v", err)
	}
}

// TestSecurityRoundTripFailsClosedForDescriptionWithTaskmgrUIToken is a round-trip
// property test: if the rendered document's description field contains any
// TASKMGRUI: token (introduced after rendering, e.g. by operator edit), parsing must
// fail closed — never silently return a result.
func TestSecurityRoundTripFailsClosedForDescriptionWithTaskmgrUIToken(t *testing.T) {
	t.Parallel()

	markerTokens := []struct {
		name  string
		token string
	}{
		{"EDITABLE:BEGIN", issueEditMarkerEditableBegin},
		{"EDITABLE:END", issueEditMarkerEditableEnd},
		{"FIELD:LABELS:BEGIN", issueEditFieldLabelsBegin},
		{"FIELD:LABELS:END", issueEditFieldLabelsEnd},
		{"FIELD:STATUS:BEGIN", issueEditFieldStatusBegin},
		{"FIELD:STATUS:END", issueEditFieldStatusEnd},
		{"FIELD:TYPE:BEGIN", issueEditFieldTypeBegin},
		{"FIELD:TYPE:END", issueEditFieldTypeEnd},
		{"FIELD:PRIORITY:BEGIN", issueEditFieldPriorityBegin},
		{"FIELD:PRIORITY:END", issueEditFieldPriorityEnd},
		{"FIELD:ASSIGNEE:BEGIN", issueEditFieldAssigneeBegin},
		{"FIELD:ASSIGNEE:END", issueEditFieldAssigneeEnd},
		{"bare TASKMGRUI: prefix", "<!-- TASKMGRUI:CUSTOM:WHATEVER -->"},
	}

	for _, tc := range markerTokens {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			issue := IssueDetail{Summary: IssueSummary{
				Title:    "Round-trip test",
				Status:   "open",
				Type:     "task",
				Priority: 2,
			}, Description: "safe description"}

			rendered := RenderIssueEditDocument(issue)

			// Splice the token into the description body after rendering.
			tampered := strings.Replace(
				rendered,
				issueEditFieldDescriptionBegin+"\nsafe description\n"+issueEditFieldDescriptionEnd,
				issueEditFieldDescriptionBegin+"\n"+tc.token+"\n"+issueEditFieldDescriptionEnd,
				1,
			)

			_, err := ParseIssueEditDocument(tampered)
			if err == nil {
				t.Fatalf("parse should have failed closed for description containing %q, got nil error", tc.name)
			}
		})
	}
}

// TestSecurityAnyFieldBodyCannotHijackALaterField verifies that the marker guard
// covers every field body, not only the description. Before the field-scanner
// change, only the description was scanned for TASKMGRUI: tokens, so a forged
// LABELS block pasted into (for example) the assignee body was returned as the
// issue's labels — silently replacing them on save.
func TestSecurityAnyFieldBodyCannotHijackALaterField(t *testing.T) {
	t.Parallel()

	forgedLabels := issueEditFieldLabelsBegin + "\nstolen\n" + issueEditFieldLabelsEnd

	cases := []struct {
		name  string
		begin string
		end   string
		body  string
	}{
		{"title", issueEditFieldTitleBegin, issueEditFieldTitleEnd, "Hijack source"},
		{"status", issueEditFieldStatusBegin, issueEditFieldStatusEnd, "open"},
		{"type", issueEditFieldTypeBegin, issueEditFieldTypeEnd, "task"},
		{"assignee", issueEditFieldAssigneeBegin, issueEditFieldAssigneeEnd, "alice"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			issue := IssueDetail{Summary: IssueSummary{
				Title:    "Hijack source",
				Status:   "open",
				Type:     "task",
				Priority: 2,
				Assignee: "alice",
				Labels:   []string{"real-label"},
			}, Description: "safe description"}

			rendered := RenderIssueEditDocument(issue)
			original := tc.begin + "\n" + tc.body + "\n" + tc.end
			if !strings.Contains(rendered, original) {
				t.Fatalf("rendered document does not contain the %s block to tamper with", tc.name)
			}

			tampered := strings.Replace(
				rendered,
				original,
				tc.begin+"\n"+tc.body+"\n"+forgedLabels+"\n"+tc.end,
				1,
			)

			doc, err := ParseIssueEditDocument(tampered)
			if err != nil {
				if !strings.Contains(err.Error(), "TASKMGRUI:") {
					t.Fatalf("expected the error to name the TASKMGRUI: token, got: %v", err)
				}
				return
			}

			// Accepting the document is only safe if the real labels survived.
			if len(doc.Labels) != 1 || doc.Labels[0] != "real-label" {
				t.Fatalf("forged markers in the %s body hijacked labels: got %v, want [real-label]", tc.name, doc.Labels)
			}
		})
	}
}
