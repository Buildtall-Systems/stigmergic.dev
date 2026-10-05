package lists

import (
	"slices"

	"github.com/nbd-wtf/go-nostr"
)

// TagsEqual compares two tag lists as ordered sequences, element by element.
// Order is part of what a writer states: a live event carrying the same tags
// in another order is a different event, republished once into the stated
// order and silent from then on. Comparing whole sequences is also what makes
// a tag a writer stopped stating visible at all: a comparison per name can
// only ever see the names it was taught, and the names it was not taught are
// exactly the ones an event loses without saying so. Lifted from btk/okf so
// every republish decision in the tree runs one comparison.
func TagsEqual(a, b nostr.Tags) bool {
	return slices.EqualFunc(a, b, func(x, y nostr.Tag) bool { return slices.Equal(x, y) })
}
