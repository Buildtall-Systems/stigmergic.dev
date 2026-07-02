package models

// OutlineEntry is one heading in a document's outline. ID matches the
// auto-generated anchor id on the corresponding heading in the rendered
// HTML, so "#" + ID is a valid in-document link.
type OutlineEntry struct {
	Text  string // flattened plain text of the heading
	ID    string // anchor id (goldmark AutoHeadingID)
	Level int    // heading level, 1-6
}
