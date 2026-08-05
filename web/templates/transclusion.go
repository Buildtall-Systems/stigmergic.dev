package templates

import "encoding/json"

// TranscludedAttr encodes the routes a page transcluded for the
// data-transcluded attribute the live-reload client reads.
//
// JSON rather than a separator-joined list: a route is a file path and may
// contain any character a separator would claim, spaces above all. An empty
// page carries an empty array rather than no attribute, so the client parses
// one shape in every case.
func TranscludedAttr(routes []string) string {
	if len(routes) == 0 {
		return "[]"
	}
	encoded, err := json.Marshal(routes)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}
