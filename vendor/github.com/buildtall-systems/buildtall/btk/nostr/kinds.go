package nostr

// KindLongForm is the addressable long-form content kind (NIP-23). Its d tag
// names the article rather than the revision, so a rewrite replaces its
// predecessor at the same coordinate.
const KindLongForm = 30023

// KindFeedCommand is the ephemeral event kind (NIP-01 ephemeral range,
// 20000-29999) that carries an on-demand feed lifecycle command from drss to
// r2n. Ephemeral events are not stored by relays, so commands neither replay on
// restart nor require dedup machinery.
const KindFeedCommand = 21337

// Feed command actions carried in the "action" tag of a KindFeedCommand event.
const (
	FeedActionBackfill = "backfill"
	FeedActionPrune    = "prune"
	FeedActionDelete   = "delete"
)

// TagAction is the tag name carrying the feed command action.
const TagAction = "action"

// TagCoordinate names a reference to an addressable event (NIP-01). Its value
// is the "kind:pubkey:d-tag" coordinate, which names an event's identity rather
// than any one revision of it.
const TagCoordinate = "a"

// Tag names a long-form event carries (NIP-23), plus the d tag every
// addressable event is named by.
const (
	TagD           = "d"
	TagTitle       = "title"
	TagSummary     = "summary"
	TagImage       = "image"
	TagPublishedAt = "published_at"
	TagTopic       = "t"
)

// KindBlossomServerList is the replaceable event kind (BUD-03) whose "server"
// tags name the Blossom servers a user's blobs should be sought on, in
// preference order. It is replaceable rather than addressable: it carries no d
// tag, and the newest event by created_at is the user's whole statement.
const KindBlossomServerList = 10063

// TagServer is the tag name carrying one server URL in a KindBlossomServerList
// event.
const TagServer = "server"
