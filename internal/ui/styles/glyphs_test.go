package styles

import (
	"strconv"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestGlyphSetMarkersAreOneCell pins the width every row and frame is laid out
// with: a marker that measures two cells shifts the title of its row.
func TestGlyphSetMarkersAreOneCell(t *testing.T) {
	t.Parallel()

	if len(GlyphSets()) != 3 {
		t.Fatalf("GlyphSets() = %v, want ascii, nerd and unicode", GlyphSets())
	}

	for _, name := range GlyphSets() {
		set := glyphSets[name]

		markers := map[string]string{
			"cursor": set.Cursor, "prompt": set.Prompt, "previous step": set.StepPrev, "next step": set.StepNext,
			"unknown issue type": set.issueTypeUnknown,
		}
		for i, frame := range set.Spinner {
			markers["spinner frame "+strconv.Itoa(i)] = frame
		}
		for token := range letterTypes {
			glyph, recognised := set.IssueType(token)
			if !recognised {
				t.Errorf("%s: issue type %q has no marker", name, token)
			}
			markers["issue type "+token] = glyph
		}
		for token, glyph := range set.status {
			markers["status "+token] = glyph
		}
		if set.status != nil {
			markers["unknown status"] = set.statusUnknown
		}
		for i, glyph := range set.priority {
			markers["priority "+strconv.Itoa(i)] = glyph
		}

		for what, glyph := range markers {
			if got := lipgloss.Width(glyph); got != 1 {
				t.Errorf("%s: %s %q is %d cells wide, want 1", name, what, glyph, got)
			}
		}

		// The shell counts ten frames; a cycle that does not divide ten jumps
		// where the count wraps.
		if n := len(set.Spinner); n == 0 || 10%n != 0 {
			t.Errorf("%s: spinner has %d frames, which does not divide 10", name, n)
		}
	}
}

func TestGlyphSetIssueVocabulary(t *testing.T) {
	t.Parallel()

	nerd, unicode := glyphSets["nerd"], glyphSets["unicode"]

	if glyph, recognised := unicode.IssueType(" Docs "); glyph != "D" || !recognised {
		t.Errorf(`unicode IssueType(" Docs ") = %q, %v; want "D", true`, glyph, recognised)
	}
	if glyph, recognised := nerd.IssueType("not-a-type"); glyph != nerd.issueTypeUnknown || recognised {
		t.Errorf(`nerd IssueType("not-a-type") = %q, %v; want the unknown marker, false`, glyph, recognised)
	}

	if _, ok := unicode.Status("open"); ok {
		t.Error("the unicode set spells a status with letters, so Status must report no icon")
	}
	if glyph, ok := nerd.Status("In-Progress"); !ok || glyph != nerd.status["in_progress"] {
		t.Errorf(`nerd Status("In-Progress") = %q, %v; want the in_progress icon`, glyph, ok)
	}
	if glyph, ok := nerd.Status("not-a-status"); !ok || glyph != nerd.statusUnknown {
		t.Errorf(`nerd Status("not-a-status") = %q, %v; want the unknown icon`, glyph, ok)
	}

	for priority, want := range map[int]string{-1: "P0", 0: "P0", 3: "P3", 7: "P7"} {
		if got := unicode.Priority(priority); got != want {
			t.Errorf("unicode Priority(%d) = %q, want %q", priority, got, want)
		}
	}
	for priority, want := range map[int]string{-1: nerd.priority[0], 0: nerd.priority[0], 3: nerd.priority[3], 4: nerd.priority[4], 7: nerd.priority[4]} {
		if got := nerd.Priority(priority); got != want {
			t.Errorf("nerd Priority(%d) = %q, want %q", priority, got, want)
		}
	}

	// The store accepts P0 to P4. Two of them behind one icon cannot be told
	// apart on a row, which the letters of the other sets never allowed.
	drawn := map[string]int{}
	for priority := 0; priority <= 4; priority++ {
		glyph := nerd.Priority(priority)
		if other, ok := drawn[glyph]; ok {
			t.Errorf("nerd draws P%d and P%d with the same icon %q", other, priority, glyph)
		}
		drawn[glyph] = priority
	}
}
