package vault

import (
	"context"
	"io/fs"
	"net/http"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/source"
)

// Source is the ContentSource over one fetched vault. It asserts Timestamped
// only: a document's created_at is a meaningful mod time. It declines
// Watchable, GitignoreAware, and Rooted, following EmbeddedSource: a vault
// is fetched at serve start, honors no gitignore, and sits at no local path.
type Source struct {
	fsys *FS
	name string
}

var (
	_ source.ContentSource = (*Source)(nil)
	_ source.Timestamped   = (*Source)(nil)
)

// NewSource wraps a fetched vault as a content source. ctx bounds the blob
// fetches the filesystem performs over the serve's lifetime; a nil client
// means http.DefaultClient.
func NewSource(ctx context.Context, v *Vault, client *http.Client) (*Source, error) {
	fsys, err := NewFS(ctx, v.Bundle, v.Name, v.docModTimes(), v.Servers, client)
	if err != nil {
		return nil, err
	}
	return &Source{fsys: fsys, name: v.Name}, nil
}

func (s *Source) FS() fs.FS    { return s.fsys }
func (s *Source) Name() string { return s.name }
func (s *Source) Close() error { return nil }

// ModTimesMeaningful asserts source.Timestamped: concept mod times come from
// document events, so the corpus staleness stamp can trust them.
func (s *Source) ModTimesMeaningful() {}
