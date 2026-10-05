package lists

import (
	"fmt"
	"strconv"

	"github.com/nbd-wtf/go-nostr"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// This file is the writer half of kind 1063 file metadata: composing a
// statement into an event under one fixed tag order, and projecting a live
// event back into a statement so the two can be compared. The reader half in
// fileitems.go projects for display; this projection decides whether a
// file's live statement still says what the writer would say now.

// FileStatement is what one kind 1063 states about a file: the fields a
// writer composes and a re-run compares. Size is bytes; Dim is the NIP-94
// widthxheight string, empty for anything that is not an image; Private is
// the BUD-11 access policy, false for public.
type FileStatement struct {
	URL     string
	MIME    string
	Hash    string
	Name    string
	Dim     string
	Size    int64
	Private bool
}

// Equal reports whether two statements say the same thing about a file, every
// field compared. A re-run reuses a live 1063 only on this equality, so a
// change to any field is a new statement.
func (s FileStatement) Equal(o FileStatement) bool {
	return s == o
}

// NewFileMetadataEvent composes an unsigned kind 1063 from a statement. The
// tag order is fixed: url, m, x, size, then dim only when set, then policy
// only when private, so the same statement always yields the same tags and
// the reader's absence-reads-as-public rule holds on the wire. The name is
// the content, first in the reader's Name of a File chain. created_at is the
// present; a caller replacing a live event restamps with NextCreatedAt.
func NewFileMetadataEvent(npub string, s FileStatement) (*nostr.Event, error) {
	pubkey, err := btknostr.NpubToHex(npub)
	if err != nil {
		return nil, fmt.Errorf("author npub: %w", err)
	}
	tags := nostr.Tags{
		{TagURL, s.URL},
		{TagMIME, s.MIME},
		{TagHash, s.Hash},
		{TagSize, strconv.FormatInt(s.Size, 10)},
	}
	if s.Dim != "" {
		tags = append(tags, nostr.Tag{TagDim, s.Dim})
	}
	if s.Private {
		tags = append(tags, nostr.Tag{TagPolicy, PolicyPrivate})
	}
	return &nostr.Event{
		Kind:      KindFileMetadata,
		PubKey:    pubkey,
		CreatedAt: nostr.Now(),
		Tags:      tags,
		Content:   s.Name,
	}, nil
}

// StatementOf projects a live kind 1063 into the statement a writer would
// compare against, through the same projection the reader uses, so a foreign
// writer's fallbacks (a name from alt or summary, a size that does not parse)
// read here exactly as they display. Any other kind is refused.
func StatementOf(ev *nostr.Event) (FileStatement, error) {
	meta, err := ProjectFileMetadata(ev)
	if err != nil {
		return FileStatement{}, err
	}
	return FileStatement{
		URL:     meta.URL,
		MIME:    meta.MIME,
		Hash:    meta.Hash,
		Name:    meta.Name,
		Dim:     meta.Dim,
		Size:    meta.Size,
		Private: meta.Private,
	}, nil
}
