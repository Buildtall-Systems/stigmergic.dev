package server

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"

	"github.com/nbd-wtf/go-nostr"
	"go.abhg.dev/goldmark/wikilink"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/markdown"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
	"github.com/Buildtall-Systems/stigmergic.dev/internal/source"
	vaultsrc "github.com/Buildtall-Systems/stigmergic.dev/internal/source/vault"
)

// VaultLoader discovers and loads every vault one owner publishes. The
// server calls it for each configured npub at startup with no signer, and
// once for each npub that signs in with a signer acting as that npub, which
// answers a relay's NIP-42 challenge so the owner's private vaults are read.
// It is injected rather than built here so the server owns no relay
// connection of its own: the command that knows the relays owns the pools,
// and a test supplies synthetic vaults with no network at all.
//
// A loader that met a challenge it could not answer returns the vaults it
// did read together with ErrAuthRequired.
type VaultLoader func(ctx context.Context, owner string, signer nostr.Signer) ([]*vaultsrc.Vault, error)

// ErrAuthRequired reports that a relay withheld events behind a NIP-42
// challenge the loader could not answer: there was no signer, or the signer
// refused or did not answer in time. The vaults read alongside it are
// complete for every relay that did answer.
var ErrAuthRequired = errors.New("a vault relay requires authentication the loader could not give")

// mount is one content source at the route prefix it answers under,
// together with everything derived from its shape: the tree the sidebar
// draws, the flat file list the corpus reads, and the resolver its own
// documents' links answer through. The derived fields are written by a scan
// and read under the server's tree mutex.
type mount struct {
	src       source.ContentSource
	watchable source.Watchable
	vault     *vaultsrc.Vault
	tree      *models.Tree
	// resolver is read on the request path while a rescan replaces it, so
	// it is held atomically rather than under the tree mutex: a render
	// needs the resolver its own scan produced, and never the lock. It
	// leads the strings and slices for field alignment, which keeps the
	// GC's pointer-scan region off the end of the struct.
	resolver atomic.Pointer[markdown.TreeResolver]
	prefix   string
	// owner is the npub a private mount belongs to: a vault read with that
	// npub's signer, which no other reader may see. Empty means public.
	owner     string
	signature string
	files     []models.SearchableFile
	ignore    []string
	caps      models.UICapabilities
}

// newMount wraps a content source at prefix, reading off what the source can
// do: one that emits events can be followed, one that filters by gitignore
// can be toggled, one with meaningful mod times can say what changed
// recently, and one anchored on the local filesystem can have its path
// copied. The empty resolver stands in until the first scan, so a request
// arriving during indexing resolves nothing rather than panicking.
func newMount(prefix string, src source.ContentSource, ignore []string) *mount {
	m := &mount{
		src:    src,
		prefix: prefix,
		tree:   &models.Tree{},
		ignore: ignore,
	}
	m.resolver.Store(markdown.NewRouteResolver(nil))
	if w, ok := src.(source.Watchable); ok {
		m.watchable = w
	}
	_, gitignoreAware := src.(source.GitignoreAware)
	_, timestamped := src.(source.Timestamped)
	_, rooted := src.(source.Rooted)
	m.caps = models.UICapabilities{
		RecentlyUpdated: timestamped,
		GitignoreToggle: gitignoreAware,
		CopyPath:        rooted,
		FollowMode:      m.watchable != nil,
	}
	return m
}

// newVaultMount serves one fetched vault under its own route namespace.
// Ignore patterns are a local filesystem's convenience and name nothing a
// vault holds, so a vault is scanned whole.
func newVaultMount(v *vaultsrc.Vault, src source.ContentSource) *mount {
	m := newMount(vaultMount(v.Owner, v.Name), src, nil)
	m.vault = v
	return m
}

// vaultMount is the route prefix one vault answers under. Owner and name
// both appear, so two owners publishing a vault of the same name mount
// side by side; the owner is the npub, which is the vault's identity
// everywhere above the wire.
func vaultMount(owner, name string) string {
	return "/vault/" + owner + "/" + name + "/"
}

// routable reports whether a descriptor's own names can form a route
// segment. A name carrying a slash would claim paths inside the mount it
// names, so it is declined rather than escaped into something the owner
// never published.
func routable(owner, name string) bool {
	return owner != "" && name != "" &&
		!strings.Contains(owner, "/") && !strings.Contains(name, "/")
}

// visibleTo reports whether viewer, a session npub or empty for an anonymous
// reader, may read this mount.
func (m *mount) visibleTo(viewer string) bool {
	return m.owner == "" || m.owner == viewer
}

// visibleMounts is the part of mounts viewer may read, in the same order.
func visibleMounts(mounts []*mount, viewer string) []*mount {
	visible := make([]*mount, 0, len(mounts))
	for _, m := range mounts {
		if m.visibleTo(viewer) {
			visible = append(visible, m)
		}
	}
	return visible
}

// hiddenPrefixes names the route prefixes of every mount viewer may not
// read. The corpus-wide caches are built once for every reader, so a reader's
// view of them is the cache with these prefixes taken out.
func hiddenPrefixes(mounts []*mount, viewer string) []string {
	var hidden []string
	for _, m := range mounts {
		if !m.visibleTo(viewer) {
			hidden = append(hidden, m.prefix)
		}
	}
	return hidden
}

// routeHidden reports whether route lies under one of the hidden prefixes.
func routeHidden(hidden []string, route string) bool {
	for _, prefix := range hidden {
		if strings.HasPrefix(route, prefix) {
			return true
		}
	}
	return false
}

// mutable reports whether a rescan can produce a different tree. A watched
// directory can, and so can one whose gitignore filtering is toggled; a
// fetched vault and an embedded site cannot, so they are scanned once, when
// they are mounted.
func (m *mount) mutable() bool {
	return m.watchable != nil || m.caps.GitignoreToggle
}

// renderSeams returns the two things rendering one of this mount's documents
// needs: the resolver answering the links it writes, and the source its
// transclusions read through. rel is the document's path inside the source.
//
// A vault matches doc-relative, so its resolver is built per document; a
// tree source's resolver depends only on the file set and is built once per
// scan.
func (m *mount) renderSeams(rel, attachmentRoot string) (wikilink.Resolver, markdown.EmbedSource) {
	if m.vault == nil {
		return m.resolver.Load(), markdown.NewFSEmbedSource(m.src.FS(), attachmentRoot)
	}

	// A path the domain cannot name resolves from the bundle root, which is
	// what an empty d-tag means to the matcher: root-relative and basename
	// matching need no location, only doc-relative does.
	dTag, err := m.vault.DocDTag(rel)
	if err != nil {
		dTag = ""
	}
	r := vaultsrc.NewResolver(m.vault, m.prefix, dTag)
	return r, vaultsrc.NewEmbedSource(m.src.FS(), r)
}

// routeEntries names documents as the corpus-wide resolver knows them: the
// path a link written inside the source would name, and the route serving
// it. Keying on the source-relative path is what lets a document in one
// source name a document in another by the name its own author would write.
func routeEntries(prefix string, files []models.SearchableFile) []markdown.RouteEntry {
	entries := make([]markdown.RouteEntry, 0, len(files))
	for _, f := range files {
		rel := strings.TrimPrefix(f.Path, "/")
		entries = append(entries, markdown.RouteEntry{Path: rel, Route: prefix + rel})
	}
	return entries
}

// routedFiles restates a source's files with their routes as paths, the form
// everything outside the mount uses: the search index, the files API, and
// every template navigate by route, and a route is the one path that means
// the same thing in a corpus holding more than one source.
func routedFiles(prefix string, files []models.SearchableFile) []models.SearchableFile {
	routed := make([]models.SearchableFile, 0, len(files))
	for _, f := range files {
		f.Path = prefix + strings.TrimPrefix(f.Path, "/")
		routed = append(routed, f)
	}
	return routed
}

// mountOf finds the mount serving a route and the path inside its source.
// The longest matching prefix wins, so a mount sitting inside another's
// namespace is found before the one containing it.
func mountOf(mounts []*mount, route string) (*mount, string, bool) {
	var best *mount
	for _, m := range mounts {
		if !strings.HasPrefix(route, m.prefix) {
			continue
		}
		if best == nil || len(m.prefix) > len(best.prefix) {
			best = m
		}
	}
	if best == nil {
		return nil, "", false
	}
	return best, strings.TrimPrefix(route, best.prefix), true
}

// mountPrefixes lists the registered route prefixes, which is how the
// backlink pass tells a destination inside the corpus from one outside it.
func mountPrefixes(mounts []*mount) []string {
	prefixes := make([]string, 0, len(mounts))
	for _, m := range mounts {
		prefixes = append(prefixes, m.prefix)
	}
	return prefixes
}
