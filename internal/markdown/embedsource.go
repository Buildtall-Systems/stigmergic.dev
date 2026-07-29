package markdown

import (
	"io/fs"
	"path"
)

// EmbedSource supplies the content behind an embed target. It is the seam
// wikilink.Resolver does not provide: a resolver answers where a target lives,
// while transclusion also needs what is there.
type EmbedSource interface {
	// NoteSource returns the raw markdown of the note at a content-root
	// relative path. A missing or unreadable note reports false rather than
	// an error, because roughly half of the link targets in a real vault
	// are dangling and an unresolved embed is an ordinary outcome.
	NoteSource(notePath string) ([]byte, bool)

	// ProbeAsset locates a non-markdown target and returns its
	// content-root relative path. A miss reports false.
	ProbeAsset(target string) (string, bool)
}

// FSEmbedSource reads embed content through a filesystem.
//
// Notes are read with fs.ReadFile, the same call handleMarkdown already makes
// for the host file, rather than through the server's search corpus. The
// corpus sits behind a mutex and can lag the filesystem during a rebuild,
// while the page being rendered was itself read from the filesystem a moment
// earlier; reading both the same way keeps a page internally consistent, and
// the OS page cache makes the reread cheap.
//
// Assets are found by probing rather than by index, which is what keeps
// source.Scan, the tree, the sidebar, and the search corpus free of binary
// entries.
type FSEmbedSource struct {
	fsys fs.FS

	// attachmentRoot is a second directory tried for bare attachment
	// filenames, which is the dominant form in an Obsidian vault. Empty
	// means only the content root is probed.
	attachmentRoot string
}

var _ EmbedSource = (*FSEmbedSource)(nil)

// NewFSEmbedSource builds an embed source over fsys, optionally probing
// attachmentRoot for targets not found at the content root.
func NewFSEmbedSource(fsys fs.FS, attachmentRoot string) *FSEmbedSource {
	return &FSEmbedSource{fsys: fsys, attachmentRoot: attachmentRoot}
}

// NoteSource reads a note's raw markdown.
func (s *FSEmbedSource) NoteSource(notePath string) ([]byte, bool) {
	if s == nil || s.fsys == nil || !fs.ValidPath(notePath) {
		return nil, false
	}
	content, err := fs.ReadFile(s.fsys, notePath)
	if err != nil {
		return nil, false
	}
	return content, true
}

// ProbeAsset tries the target as given relative to the content root, then, if
// an attachment root is configured, the target joined onto it. First hit wins.
func (s *FSEmbedSource) ProbeAsset(target string) (string, bool) {
	if s == nil || s.fsys == nil {
		return "", false
	}
	if candidate, ok := s.statFile(target); ok {
		return candidate, true
	}
	if s.attachmentRoot == "" {
		return "", false
	}
	return s.statFile(path.Join(s.attachmentRoot, target))
}

// statFile reports candidate unchanged when it names a regular file inside the
// content root. fs.ValidPath rejects absolute paths and any ".." element, so a
// target cannot escape.
func (s *FSEmbedSource) statFile(candidate string) (string, bool) {
	cleaned := path.Clean(candidate)
	if !fs.ValidPath(cleaned) {
		return "", false
	}
	info, err := fs.Stat(s.fsys, cleaned)
	if err != nil || info.IsDir() {
		return "", false
	}
	return cleaned, true
}
