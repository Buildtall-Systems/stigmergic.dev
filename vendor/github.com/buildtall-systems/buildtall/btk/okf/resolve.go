package okf

import (
	"path"
	"strings"

	"github.com/nbd-wtf/go-nostr"

	"github.com/buildtall-systems/buildtall/btk/lists"
	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// Render-time resolution: an okf-aware consumer joins a link target to the
// statements the vault's sets already carry, so links resolve with zero new
// wire state and document content stays byte-verbatim. The resolver builds
// the same file index the publish-side graph uses, from set events instead
// of a Bundle, which is what keeps the two sides resolving one link one way.

// ResolutionKind says what a resolved link target is on the wire.
type ResolutionKind int

const (
	ResolvedDocument ResolutionKind = iota
	ResolvedAttachment
)

// Resolution is a resolved link target. A document resolution carries the
// member coordinate and the reference's fragment; routing either is the
// consumer's business, and Path hands that business the matched entry's
// bundle-relative path so no consumer re-derives it from the coordinate. An
// attachment resolution carries the blob URL its bytes are served under, and
// its Path names the file inside the bundle.
type Resolution struct {
	Coordinate string
	Fragment   string
	BlobURL    string
	Path       string
	// Kind sits last for field alignment: its non-pointer word keeps the
	// GC's pointer-scan region off the end of the struct.
	Kind ResolutionKind
}

// Resolver answers link targets from a vault's set events alone: no bundle,
// no filesystem, no fetch beyond the sets a consumer already holds.
type Resolver struct {
	idx    *fileIndex
	coords map[string]string
	base   string
	domain lists.Domain
}

// NewResolver indexes the vault's set events for resolution. Each set's
// okf-attachment statements become files in the directory the set's d-tag
// names, the reserved root set's in the bundle root; each set's member
// documents, the owner's long-form coordinates, become documents at the path
// their d-tag derives. base is the Blossom base URL attachment resolutions
// are formed against. A statement that parses to nothing indexes nothing: at
// render time an unresolvable link is a dangling report, never a refusal.
func NewResolver(domain lists.Domain, sets map[string]*nostr.Event, base string) *Resolver {
	r := &Resolver{
		idx:    newFileIndex(),
		coords: map[string]string{},
		domain: domain,
		base:   strings.TrimRight(base, "/"),
	}
	rootSetDTag := domain.RootDTag + domain.CompanionSuffix
	for _, ev := range sets {
		if ev == nil {
			continue
		}
		setDTag := lists.GetDTag(ev)
		var dirPath string
		if setDTag != rootSetDTag {
			p, err := DTagToPath(domain, setDTag)
			if err != nil {
				continue
			}
			dirPath = p
		}
		for _, tag := range ev.Tags {
			r.indexTag(dirPath, ev.PubKey, tag)
		}
	}
	return r
}

// indexTag files one set statement: an attachment into the set's directory,
// or an owner document at its derived path. A citation, a member coordinate
// authored by someone else, indexes nothing, because a vault's files are the
// owner's; a child-set member indexes nothing here, because the child set
// states its own holdings.
func (r *Resolver) indexTag(dirPath, ownerHex string, tag nostr.Tag) {
	switch {
	case len(tag) >= 3 && tag[0] == TagOKFAttachment:
		if tag[1] == "" || !isBlobHash(tag[2]) {
			return
		}
		r.idx.add(FileEntry{Path: joinPath(dirPath, tag[1]), SHA256: tag[2]})
	case len(tag) >= 2 && tag[0] == btknostr.TagCoordinate:
		kind, pubkeyHex, dTag, err := btknostr.ParseCoordinate(tag[1])
		if err != nil || kind != lists.KindLongFormNote || pubkeyHex != ownerHex {
			return
		}
		docPath, err := DTagToPath(r.domain, dTag)
		if err != nil {
			return
		}
		filePath := docPath + conceptFileExt
		r.idx.add(FileEntry{Path: filePath})
		r.coords[filePath] = tag[1]
	}
}

// Resolve answers one link reference from the referencing document's d-tag.
// The d-tag locates the document in the tree for doc-relative matching; a
// d-tag naming no path in the domain resolves from the bundle root, since
// the root-relative and basename tiers need no location. Unresolved is a
// not-found, never an error: dangling is a report, not a refusal.
func (r *Resolver) Resolve(docDTag string, ref LinkRef) (Resolution, bool) {
	var refDir string
	if docPath, err := DTagToPath(r.domain, docDTag); err == nil {
		refDir, _ = splitPath(docPath)
	}
	entry, ok := r.idx.resolve(refDir, ref)
	if !ok {
		return Resolution{}, false
	}
	if entry.SHA256 != "" {
		return Resolution{Kind: ResolvedAttachment, BlobURL: r.blobURL(entry), Path: entry.Path}, true
	}
	return Resolution{Kind: ResolvedDocument, Coordinate: r.coords[entry.Path], Fragment: ref.Fragment, Path: entry.Path}, true
}

// blobURL forms the house blob URL: the base joined by exactly one slash to
// the lowercase hex hash, plus a dot and the file's extension when the name
// carries an alphanumeric one (BUD-01: servers MUST accept that form), bare
// hash otherwise. The join normalizes here because the recorded double-slash
// bug class comes from trailing-slash bases.
func (r *Resolver) blobURL(e FileEntry) string {
	_, name := splitPath(e.Path)
	url := r.base + "/" + e.SHA256
	if ext := alnumExt(name); ext != "" {
		url += "." + ext
	}
	return url
}

// alnumExt returns name's extension without its dot when that extension is
// nonempty and alphanumeric, empty otherwise.
func alnumExt(name string) string {
	ext := path.Ext(name)
	if len(ext) < 2 {
		return ""
	}
	ext = ext[1:]
	for i := 0; i < len(ext); i++ {
		if !isAlnum(ext[i]) {
			return ""
		}
	}
	return ext
}

func isAlnum(c byte) bool {
	return isAlpha(c) || (c >= '0' && c <= '9')
}
