// Package site holds the public stigmergic.dev website content, compiled
// into the binary. Content changes ship only through deliberate commits and
// tagged releases.
package site

import (
	"embed"
	"io/fs"
)

//go:embed all:content
var content embed.FS

// Content returns the embedded website tree rooted at the content directory.
func Content() (fs.FS, error) {
	return fs.Sub(content, "content")
}
