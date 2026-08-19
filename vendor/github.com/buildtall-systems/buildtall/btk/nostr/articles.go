package nostr

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

const (
	// articleBatchTimeout bounds one author's batch in FetchLongFormByAddress.
	// Each batch is a single addressable query, so a relay that has not answered
	// within it is not going to.
	articleBatchTimeout = 30 * time.Second

	// publishedAtDate is the calendar form some clients write into published_at
	// in place of the unix seconds NIP-23 specifies. Both are read.
	publishedAtDate = "2006-01-02"
)

// Article is a kind 30023 long-form post reduced to the fields buildtall
// services carry: comprehend indexes them, btcli vault writes them as OKF
// concepts.
type Article struct {
	ID             string
	Pubkey         string
	DTag           string
	Title          string
	Summary        string
	Content        string
	PublishedAt    *time.Time
	EventCreatedAt time.Time
	ImageURL       string
	Tags           []string
}

// ArticleAddress names one addressable article by author and d tag: the two
// halves of a 30023 coordinate that vary once the kind is fixed.
type ArticleAddress struct {
	Pubkey string
	DTag   string
}

// AddressesFromCoordinates converts harvested coordinate strings to article
// addresses, admitting only kind 30023: a malformed coordinate and a foreign
// kind are each logged and skipped, never fetched. log must be non-nil.
func AddressesFromCoordinates(coords []string, log *slog.Logger) []ArticleAddress {
	addrs := make([]ArticleAddress, 0, len(coords))
	for _, coord := range coords {
		kind, pubkey, dTag, err := ParseCoordinate(coord)
		if err != nil {
			log.Warn("skipping malformed article coordinate", "coord", coord, "error", err)
			continue
		}
		if kind != KindLongForm {
			log.Warn("skipping non-article coordinate", "coord", coord, "kind", kind)
			continue
		}
		addrs = append(addrs, ArticleAddress{Pubkey: pubkey, DTag: dTag})
	}
	return addrs
}

// MissingArticleAddresses reports the requested addresses no fetched event
// answers, in request order: the reading room's availability census.
func MissingArticleAddresses(addrs []ArticleAddress, events []*nostr.Event) []ArticleAddress {
	got := make(map[ArticleAddress]bool, len(events))
	for _, ev := range events {
		if ev.Kind != KindLongForm {
			continue
		}
		got[ArticleAddress{Pubkey: ev.PubKey, DTag: ev.Tags.GetD()}] = true
	}
	var missing []ArticleAddress
	for _, addr := range addrs {
		if !got[addr] {
			missing = append(missing, addr)
		}
	}
	return missing
}

// EventToArticle projects a long-form event onto Article. It returns nil for
// any other kind, and for a 30023 carrying no d tag, which no a tag can address
// and no rewrite can replace.
func EventToArticle(ev *nostr.Event) *Article {
	if ev.Kind != KindLongForm {
		return nil
	}

	a := &Article{
		Pubkey:         ev.PubKey,
		Content:        ev.Content,
		EventCreatedAt: ev.CreatedAt.Time(),
	}

	for _, tag := range ev.Tags {
		if len(tag) < 2 {
			continue
		}
		switch tag[0] {
		case TagD:
			a.DTag = tag[1]
		case TagTitle:
			a.Title = tag[1]
		case TagSummary:
			a.Summary = tag[1]
		case TagImage:
			a.ImageURL = tag[1]
		case TagPublishedAt:
			if ts, err := time.Parse(publishedAtDate, tag[1]); err == nil {
				a.PublishedAt = &ts
			} else if ts, err := unixSeconds(tag[1]); err == nil {
				a.PublishedAt = &ts
			}
		case TagTopic:
			a.Tags = append(a.Tags, tag[1])
		}
	}

	if a.DTag == "" {
		return nil
	}
	a.ID = fmt.Sprintf("%d:%s:%s", KindLongForm, a.Pubkey, a.DTag)
	return a
}

// FetchLongFormByAddress fetches the addressed long-form events, batching one
// filter per author so a relay answers a whole d tag set in one query. Reads go
// through pool, so a caller that authenticated it keeps that session, and the
// caller owns the pool's lifetime. log must be non-nil.
//
// The events arrive whole. A caller that will republish one needs every tag it
// carries, and Article keeps six; a caller that only reads should take the
// projection from FetchArticlesByAddress instead.
//
// The returned error exists for callers to switch on and is always nil today:
// FetchManyReplaceable discards the reason a subscription ended, so a relay
// that failed and a document that does not exist are the same absence here.
func FetchLongFormByAddress(ctx context.Context, pool *nostr.SimplePool, relays []string, addrs []ArticleAddress, log *slog.Logger) ([]*nostr.Event, error) {
	batches := make(map[string][]string)
	for _, addr := range addrs {
		batches[addr.Pubkey] = append(batches[addr.Pubkey], addr.DTag)
	}

	events := make([]*nostr.Event, 0, len(addrs))
	for pubkey, dtags := range batches {
		filter := nostr.Filter{
			Authors: []string{pubkey},
			Kinds:   []int{KindLongForm},
			Tags:    nostr.TagMap{"d": dtags},
		}

		batchCtx, cancel := context.WithTimeout(ctx, articleBatchTimeout)
		results := pool.FetchManyReplaceable(batchCtx, relays, filter)
		cancel()

		found := 0
		results.Range(func(_ nostr.ReplaceableKey, ev *nostr.Event) bool {
			events = append(events, ev)
			found++
			return true
		})

		log.Debug("fetched long-form documents for author",
			"npub", authorLabel(pubkey),
			"requested", len(dtags),
			"found", found,
		)
	}

	return events, nil
}

// FetchArticlesByAddress fetches the addressed articles and projects each onto
// Article, dropping whatever the projection cannot represent. It is the read
// path: republishing what it returns would strip every tag Article does not
// keep.
func FetchArticlesByAddress(ctx context.Context, pool *nostr.SimplePool, relays []string, addrs []ArticleAddress, log *slog.Logger) ([]Article, error) {
	events, err := FetchLongFormByAddress(ctx, pool, relays, addrs, log)
	if err != nil {
		return nil, err
	}

	articles := make([]Article, 0, len(events))
	for _, ev := range events {
		if a := EventToArticle(ev); a != nil {
			articles = append(articles, *a)
		}
	}
	return articles, nil
}

// authorLabel renders a hex pubkey as an npub for logs, falling back to the hex
// when the key will not encode: a log line is not the place to fail a fetch.
func authorLabel(pubkey string) string {
	npub, err := HexToNpub(pubkey)
	if err != nil {
		return pubkey
	}
	return npub
}

func unixSeconds(s string) (time.Time, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(n, 0), nil
}
