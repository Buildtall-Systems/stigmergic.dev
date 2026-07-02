package source

import "io/fs"

// EmbeddedSource serves content compiled into the binary. It asserts none of
// the optional capabilities: the content never changes at runtime, carries no
// meaningful mod times, and has no local filesystem root.
type EmbeddedSource struct {
	fsys fs.FS
	name string
}

var _ ContentSource = (*EmbeddedSource)(nil)

// NewEmbedded wraps fsys, an already-rooted content tree, under a
// human-readable name for logging and display.
func NewEmbedded(fsys fs.FS, name string) *EmbeddedSource {
	return &EmbeddedSource{fsys: fsys, name: name}
}

func (e *EmbeddedSource) FS() fs.FS {
	return e.fsys
}

func (e *EmbeddedSource) Name() string {
	return e.name
}

// Close is a no-op: embedded content holds no resources.
func (e *EmbeddedSource) Close() error {
	return nil
}
