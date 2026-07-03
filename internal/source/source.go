// Package source abstracts where rendered content comes from. A ContentSource
// exposes a rooted fs.FS; optional behaviors (live change events, gitignore
// awareness, meaningful mod times, a local root path) are advertised by
// asserting the capability interfaces below.
package source

import "io/fs"

// ContentSource is the minimal contract every content provider satisfies.
// The returned fs.FS is rooted at the content tree; fs.ValidPath semantics
// provide traversal protection by construction.
type ContentSource interface {
	FS() fs.FS
	Name() string
	Close() error
}

// Event is a content change already classified as relevant to rendering
// (a markdown file or a directory). Consumers never need to stat the path.
// Path is corpus-relative with forward slashes (fs.ValidPath semantics),
// matching the paths served under /file/.
type Event struct {
	Path string
}

// Watchable is asserted by sources that emit live change events.
// Both channels are closed when the source is closed.
type Watchable interface {
	Events() <-chan Event
	Errors() <-chan error
}

// GitignoreAware is asserted by sources that honor .gitignore filtering and
// can toggle it at runtime. ToggleGitignore returns the new state.
type GitignoreAware interface {
	RespectingGitignore() bool
	ToggleGitignore() bool
}

// Timestamped is asserted by sources whose fs.Stat mod times are meaningful.
type Timestamped interface {
	ModTimesMeaningful()
}

// Rooted is asserted by sources anchored at an absolute path on the local
// filesystem.
type Rooted interface {
	Root() string
}
