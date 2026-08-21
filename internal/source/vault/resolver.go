package vault

import (
	"io/fs"
	"net/url"
	"strings"

	"go.abhg.dev/goldmark/wikilink"

	"github.com/buildtall-systems/buildtall/btk/okf"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/markdown"
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

var (
	_ wikilink.Resolver        = (*Resolver)(nil)
	_ markdown.RouteResolver   = (*Resolver)(nil)
	_ markdown.AbsenceReporter = (*Resolver)(nil)
)

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
	route, ok := r.route(okf.LinkRef{
		Target:   target,
		Fragment: string(n.Fragment),
		Embed:    n.Embed,
		Syntax:   okf.SyntaxWikilink,
	})
	if !ok {
		return nil, nil
	}
	return []byte(route), nil
}

// ResolveRoute answers a plain CommonMark destination through the same
// matcher, which is what keeps a markdown link and a wikilink to the same
// concept pointing at one route. The destination is percent-decoded before
// matching, because the name in the vault is the reference and a foreign
// corpus writes "my%20note.md" for "my note.md"; the anchor splits off
// first, so an escaped "%23" in a name never masquerades as one.
func (r *Resolver) ResolveRoute(target string) (string, bool) {
	name, fragment, _ := strings.Cut(target, "#")
	return r.route(okf.LinkRef{
		Target:   pathUnescaped(name),
		Fragment: pathUnescaped(fragment),
		Syntax:   okf.SyntaxLink,
	})
}

// RouteAbsent declares a target the vault does not hold. The vault may say
// so where the local corpus cannot: its index carries okf-attachment
// statements beside the member documents, so a name that resolves to nothing
// names nothing the vault holds.
func (r *Resolver) RouteAbsent(target string) bool {
	_, ok := r.ResolveRoute(target)
	return !ok
}

// route resolves one reference and forms the in-mount route for what it
// names. A document keeps the reference's fragment; an attachment routes to
// its path inside the mount so the bytes serve through the filesystem.
func (r *Resolver) route(ref okf.LinkRef) (string, bool) {
	res, ok := r.vault.Resolver.Resolve(r.docDTag, ref)
	if !ok {
		return "", false
	}
	route := r.mount + "/" + res.Path
	if res.Kind == okf.ResolvedDocument && res.Fragment != "" {
		route += "#" + res.Fragment
	}
	return route, true
}

// pathUnescaped percent-decodes s, leaving bytes that are not valid
// percent-encoding literal: an author who wrote a stray "%" wrote a "%".
func pathUnescaped(s string) string {
	decoded, err := url.PathUnescape(s)
	if err != nil {
		return s
	}
	return decoded
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
// its path inside the vault, where the filesystem serves its bytes. The
// path is bundle-relative, matching what NoteSource reads and what the
// renderer prefixes with the mount to form the link it writes.
func (e *EmbedSource) ProbeAsset(target string) (string, bool) {
	ref := okf.LinkRef{Target: target, Embed: true, Syntax: okf.SyntaxWikilink}
	res, ok := e.resolver.vault.Resolver.Resolve(e.resolver.docDTag, ref)
	if !ok || res.Kind != okf.ResolvedAttachment {
		return "", false
	}
	return res.Path, true
}
