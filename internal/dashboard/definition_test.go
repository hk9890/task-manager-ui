package dashboard

import (
	"strings"
	"testing"
)

func TestValidateDefinitions(t *testing.T) {
	t.Parallel()

	valid := []Definition{
		{
			ID:    "default",
			Title: "Default",
			Sections: []Section{
				{ID: "ready", Title: "Ready"},
				{ID: "blocked", Title: "Blocked"},
			},
		},
		{
			ID:    "secondary",
			Title: "Secondary",
			Sections: []Section{
				{ID: "in_progress", Title: "In Progress"},
			},
		},
	}

	tests := []struct {
		name     string
		defs     []Definition
		wantErr  bool
		errMatch string
	}{
		{name: "valid multi dashboard", defs: valid},
		{name: "zero definitions", defs: nil, wantErr: true, errMatch: "zero definitions"},
		{name: "empty dashboard id", defs: []Definition{{Title: "Default", Sections: []Section{{ID: "ready", Title: "Ready"}}}}, wantErr: true, errMatch: "id is required"},
		{name: "empty dashboard title", defs: []Definition{{ID: "default", Sections: []Section{{ID: "ready", Title: "Ready"}}}}, wantErr: true, errMatch: "title is required"},
		{name: "zero sections", defs: []Definition{{ID: "default", Title: "Default"}}, wantErr: true, errMatch: "at least one section is required"},
		{name: "empty section id", defs: []Definition{{ID: "default", Title: "Default", Sections: []Section{{Title: "Ready"}}}}, wantErr: true, errMatch: "section[0]: id is required"},
		{name: "empty section title", defs: []Definition{{ID: "default", Title: "Default", Sections: []Section{{ID: "ready"}}}}, wantErr: true, errMatch: "section[0]: title is required"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDefinitions(tc.defs)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected validation error")
				}
				if tc.errMatch != "" && !strings.Contains(err.Error(), tc.errMatch) {
					t.Fatalf("expected error containing %q, got %q", tc.errMatch, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("expected valid definitions, got error: %v", err)
			}
		})
	}
}
