package models

// BacklinkEntry represents a single file that links to the current document.
type BacklinkEntry struct {
	SourcePath  string // route path (e.g., "folder/note.md")
	SourceTitle string // display name (filename sans .md)
}

// BacklinkIndex maps normalized target routes to their inbound link sources.
type BacklinkIndex map[string][]BacklinkEntry
