package models

import "testing"

// ShortNpub abbreviates by length alone, so a value shorter than the label
// it would produce comes back whole rather than padded into something longer
// than it began. Real npubs are exercised where real keys are derived, in
// the server's vault tests; what is pinned here is the boundary.
func TestShortNpubLeavesAShortValueWhole(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"", "short", "shorter than the label"[:npubHead+npubTail+1]} {
		if got := ShortNpub(value); got != value {
			t.Errorf("expected %q to come back whole, got %q", value, got)
		}
	}
}

// A value one character past the boundary abbreviates, and the abbreviation
// is shorter than what it replaced: shortening that lengthened would defeat
// the label it exists to fit.
func TestShortNpubAbbreviatesPastTheBoundary(t *testing.T) {
	t.Parallel()

	value := "a value long enough to abbreviate down to a label"
	got := ShortNpub(value)
	if got == value {
		t.Fatal("expected a value past the boundary to abbreviate")
	}
	if len(got) >= len(value) {
		t.Errorf("expected %q to be shorter than %q", got, value)
	}
}
