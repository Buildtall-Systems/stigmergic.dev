package vault

import (
	"io/fs"
	"strings"

	"go.abhg.dev/goldmark/wikilink"

	"github.com/buildtall-systems/buildtall/btk/okf"
)

// Resolver adapts okf.Resolver to wikilink.Resolver for one document render.
// It knows the vault, the mount prefix, and the referencing document's
// d-tag, and forms every route inside the mount: a document keeps the
// reference's fragment, an attachment routes to its path inside the mount so
// the bytes serve through the filesystem. An unresolved target returns a nil
// destination, which renders as a visible unresolved span.
type Resolver struct {
	vault   *Vault
	mount   string
	docDTag string
}

var _ wikilink.Resolver = (*Resolver)(nil)

// NewResolver builds the adapter for one render: the mount is the source's
// route prefix, the docDTag the referencing document's identity.
func NewResolver(v *Vault, mount, docDTag string) *Resolver {
	return &Resolver{vault: v, mount: strings.TrimRight(mount, "/"), docDTag: docDTag}
}

// ResolveWikilink answers one wikilink through the okf matcher. An empty
// target with a fragment names a section of the host document.
func (r *Resolver) ResolveWikilink(n *wikilink.Node) ([]byte, error) {
	target := string(n.Target)
	if target == "" {
		if len(n.Fragment) > 0 {
			return []byte("#" + string(n.Fragment)), nil
		}
		return nil, nil
	}
	ref := okf.LinkRef{
		Target:   target,
		Fragment: string(n.Fragment),
		Embed:    n.Embed,
		Syntax:   okf.SyntaxWikilink,
	}
	res, ok := r.vault.Resolver.Resolve(r.docDTag, ref)
	if !ok {
		return nil, nil
	}
	route := r.mount + "/" + res.Path
	if res.Kind == okf.ResolvedDocument && res.Fragment != "" {
		route += "#" + res.Fragment
	}
	return []byte(route), nil
}

// EmbedSource serves transclusion for one vault render: note bytes through
// the filesystem, asset routes through the same matcher links resolve by, so
// the two kinds of embed agree with link resolution about what exists.
type EmbedSource struct {
	fsys     fs.FS
	resolver *Resolver
}

// NewEmbedSource pairs the vault's filesystem with the render's resolver.
func NewEmbedSource(fsys fs.FS, r *Resolver) *EmbedSource {
	return &EmbedSource{fsys: fsys, resolver: r}
}

// NoteSource reads the note at the given route through the filesystem. The
// route may carry the mount prefix or be bundle-relative; a missing or
// unreadable note reports false, the ordinary fate of a dangling target.
func (e *EmbedSource) NoteSource(notePath string) ([]byte, bool) {
	rel := strings.TrimPrefix(notePath, e.resolver.mount+"/")
	rel = strings.TrimPrefix(rel, "/")
	data, err := fs.ReadFile(e.fsys, rel)
	if err != nil {
		return nil, false
	}
	return data, true
}

// ProbeAsset resolves a non-markdown target through the matcher and returns
// its in-mount route, where the filesystem serves its bytes.
func (e *EmbedSource) ProbeAsset(target string) (string, bool) {
	ref := okf.LinkRef{Target: target, Embed: true, Syntax: okf.SyntaxWikilink}
	res, ok := e.resolver.vault.Resolver.Resolve(e.resolver.docDTag, ref)
	if !ok || res.Kind != okf.ResolvedAttachment {
		return "", false
	}
	return e.resolver.mount + "/" + res.Path, true
}
