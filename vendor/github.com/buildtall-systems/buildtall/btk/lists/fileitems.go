package lists

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/nbd-wtf/go-nostr"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// This file is the reader-side half of the kind 30102 file set specification
// (operations repo, nuds/list-of-lists.md): resolving a set's items into their
// kind 1063 metadata, projecting that metadata for display, and deduplicating
// merged sets by the hash the metadata commits to. The write-side half of
// the set lives in validate.go and events.go; the kind 1063 writer half, the
// composer and the statement comparison, lives in filemetadata.go.

// The NIP-94 tag names a kind 1063 carries, exported so the reader here and
// the writer in filemetadata.go spell them once, and so cmd/files can retire
// its duplicates. TagPolicy and PolicyPrivate name the BUD-11 access policy
// the uploader stamped on the event; absence reads as public, mirroring the
// store's own convention that a blob without a policy row is public.
const (
	TagURL        = "url"
	TagMIME       = "m"
	TagHash       = "x"
	TagSize       = "size"
	TagDim        = "dim"
	TagPolicy     = "policy"
	PolicyPrivate = "private"
)

const (
	// tagAlt and tagSummary are the NIP-94 fallback tags the Name of a File
	// chain reads.
	tagAlt     = "alt"
	tagSummary = "summary"
	// fileFetchBatch caps the ids one relay filter carries, per the estate's
	// relay batch discipline.
	fileFetchBatch = 50
	// truncatedHashLength is how much of an x hash stands in for a name when
	// the metadata offers nothing human. Twelve hex characters distinguish
	// files without dominating a listing.
	truncatedHashLength = 12
)

// FileItemState classifies one item of a file set after resolution. The zero
// value is unresolved: an item nobody fetched yet claims nothing.
type FileItemState int

const (
	// FileItemUnresolved names an item whose event was not found. It is a
	// state, never an error: the reader renders it as unresolved, and a
	// rewrite of the set carries it through unchanged.
	FileItemUnresolved FileItemState = iota
	// FileItemResolved names an item whose id reached a kind 1063 event.
	FileItemResolved
	// FileItemIgnored names an item that resolved to some other kind, which
	// list-of-lists.md directs the reader to ignore while keeping the rest of the
	// list.
	FileItemIgnored
)

// FileItem is one item of a file set after resolution. Item is the underlying
// list item, carried unchanged whatever the state, so a rewrite of the set
// reproduces the original tag byte for byte.
type FileItem struct {
	Meta  *FileMetadata
	Item  Item
	State FileItemState
}

// FileMetadata is the display projection of one kind 1063 event, its tags
// read directly per NIP-94. EventID is the hex event id, a protocol string
// like Item.Value; AuthorNpub is npub-canonical for the foreign-author
// attribution list-of-lists.md asks of readers.
type FileMetadata struct {
	EventID    string
	AuthorNpub string
	Name       string
	URL        string
	MIME       string
	Hash       string
	Dim        string
	Thumb      string
	Image      string
	Summary    string
	Alt        string
	Size       int64
	CreatedAt  int64
	Private    bool
}

// ProjectFileMetadata projects a kind 1063 event for display. Any other kind
// is refused: the caller decides whether that means ignoring an item.
func ProjectFileMetadata(ev *nostr.Event) (*FileMetadata, error) {
	if ev == nil {
		return nil, fmt.Errorf("event is nil")
	}
	if ev.Kind != KindFileMetadata {
		return nil, fmt.Errorf("kind %d is not file metadata (kind %d)", ev.Kind, KindFileMetadata)
	}
	npub, err := btknostr.HexToNpub(ev.PubKey)
	if err != nil {
		return nil, fmt.Errorf("encoding author npub: %w", err)
	}
	hash := firstTagValue(ev, TagHash)
	return &FileMetadata{
		EventID:    ev.ID,
		AuthorNpub: npub,
		Name:       fileName(ev, hash),
		URL:        firstTagValue(ev, TagURL),
		MIME:       firstTagValue(ev, TagMIME),
		Hash:       hash,
		Dim:        firstTagValue(ev, TagDim),
		Thumb:      firstTagValue(ev, "thumb"),
		Image:      firstTagValue(ev, "image"),
		Summary:    firstTagValue(ev, tagSummary),
		Alt:        firstTagValue(ev, tagAlt),
		Size:       parseFileSize(firstTagValue(ev, TagSize)),
		CreatedAt:  int64(ev.CreatedAt),
		Private:    firstTagValue(ev, TagPolicy) == PolicyPrivate,
	}, nil
}

// fileName applies the Name of a File fallback chain: content, then alt, then
// summary, then a truncated x hash.
func fileName(ev *nostr.Event, hash string) string {
	if ev.Content != "" {
		return ev.Content
	}
	if alt := firstTagValue(ev, tagAlt); alt != "" {
		return alt
	}
	if summary := firstTagValue(ev, tagSummary); summary != "" {
		return summary
	}
	if len(hash) > truncatedHashLength {
		return hash[:truncatedHashLength]
	}
	return hash
}

// parseFileSize tolerates a foreign writer's size tag: anything that is not a
// positive integer of bytes reads as absent.
func parseFileSize(s string) int64 {
	size, err := strconv.ParseInt(s, 10, 64)
	if err != nil || size < 0 {
		return 0
	}
	return size
}

// ResolveFileItems resolves a file set's items against the given relays:
// batched fetches by id, then the spec's read rules. The fetch carries no
// kind filter, because absence and a wrong-kind referent are different facts:
// the first leaves an item unresolved, the second ignores it. A fetch failure
// is logged and leaves its items unresolved; the caller always receives one
// FileItem per input item, in input order. An item whose type is not "e"
// names nothing resolvable and is ignored.
//
// A zero timeout takes DefaultResolveTimeout and a negative one removes the
// deadline, leaving the caller's own context as the only bound. The bound is
// the caller's to state for the same reason ResolvePolicy exists: btk serves
// a web page and a headless reader, and they do not want the same answer.
func ResolveFileItems(ctx context.Context, q CoordQuerier, items []Item, relays []string, timeout time.Duration) []FileItem {
	if timeout == 0 {
		timeout = DefaultResolveTimeout
	}
	rctx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		rctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	out := make([]FileItem, len(items))
	var ids []string
	indexesByID := make(map[string][]int)
	for i, item := range items {
		out[i] = FileItem{Item: item}
		if !item.IsEvent() {
			out[i].State = FileItemIgnored
			continue
		}
		if len(indexesByID[item.Value]) == 0 {
			ids = append(ids, item.Value)
		}
		indexesByID[item.Value] = append(indexesByID[item.Value], i)
	}

	for start := 0; start < len(ids); start += fileFetchBatch {
		batch := ids[start:min(start+fileFetchBatch, len(ids))]
		events, err := q.QueryBlocking(rctx, nostr.Filter{IDs: batch}, relays)
		if err != nil {
			slog.Warn("file items: fetch failed", "ids", len(batch), "err", err)
			continue
		}
		for _, ev := range events {
			for _, i := range indexesByID[ev.ID] {
				if out[i].State != FileItemUnresolved {
					continue
				}
				if ev.Kind != KindFileMetadata {
					out[i].State = FileItemIgnored
					continue
				}
				meta, err := ProjectFileMetadata(ev)
				if err != nil {
					slog.Warn("file items: projection failed", "id", ev.ID, "err", err)
					continue
				}
				out[i].State = FileItemResolved
				out[i].Meta = meta
			}
		}
	}

	return out
}

// DedupeFileItemsByHash removes duplicate files from a merged view: two
// resolved items whose metadata commits to the same x hash describe one file,
// and the first kept wins. Unresolved and ignored items commit to nothing, so
// they are never removed. list-of-lists.md scopes this rule to merged sets; a
// single set refuses duplicate ids at write time instead.
func DedupeFileItemsByHash(items []FileItem) []FileItem {
	seen := make(map[string]bool, len(items))
	out := make([]FileItem, 0, len(items))
	for _, item := range items {
		if item.State == FileItemResolved && item.Meta != nil && item.Meta.Hash != "" {
			if seen[item.Meta.Hash] {
				continue
			}
			seen[item.Meta.Hash] = true
		}
		out = append(out, item)
	}
	return out
}
