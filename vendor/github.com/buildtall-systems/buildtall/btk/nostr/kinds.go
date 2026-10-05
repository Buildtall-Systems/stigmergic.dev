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

// Room kinds (docs/operations/nuds/room.md) live in the same NIP-01 ephemeral
// range: a relay broadcasts them to matching subscribers and stores neither.
// Presence is a visitor's beat in a room; signaling carries one WebRTC
// handshake message to one peer.
const (
	KindRoomPresence  = 20101
	KindRoomSignaling = 20102
)

// KindKeyEscrow carries one nsec, NIP-44 encrypted to the key its p tag
// names, under a d tag of the escrowed key's own hex pubkey. drss escrows
// a feed key to r2n and a show key to the podcast producer with it; an
// escrow with no p tag predates the recipient tag and is r2n's.
const KindKeyEscrow = 31338

// KindShowDefinition is a podcast show defined at runtime, addressable by
// its slug; the podcast producer runs every active one.
const KindShowDefinition = 31341

// TagPubkey names a key an event concerns: the recipient of an escrow, the
// key of a show, the requester of a job.
const TagPubkey = "p"

// TagNonce carries a fresh value on every room event. Two identical beats in
// the same second would otherwise hash to the same id, and a reading pool
// discards the second as a duplicate.
const TagNonce = "nonce"

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
// addressable event is named by. The r tag is the web address the article
// is published at when it republishes a web page.
const (
	TagD           = "d"
	TagTitle       = "title"
	TagSummary     = "summary"
	TagImage       = "image"
	TagPublishedAt = "published_at"
	TagTopic       = "t"
	TagWebAddress  = "r"
)

// OrderPublishedAt is the opt-in filter order value our relay honors. A filter
// that carries it moves selection, bounds, and ordering onto the publication
// axis, so a since bound reads published_at rather than created_at (see the
// relay README's Publication-Order Filter Extension section). A filter that
// also carries no limit is answered with the whole matching set.
const OrderPublishedAt = "published_at"

// KindBlossomServerList is the replaceable event kind (BUD-03) whose "server"
// tags name the Blossom servers a user's blobs should be sought on, in
// preference order. It is replaceable rather than addressable: it carries no d
// tag, and the newest event by created_at is the user's whole statement.
const KindBlossomServerList = 10063

// TagServer is the tag name carrying one server URL in a KindBlossomServerList
// event.
const TagServer = "server"

// KindTrustedAssertion is the NIP-85 user-subject trusted-assertion kind. Its
// d tag names the subject, so one coordinate holds an author's current
// statement about one key and a new statement replaces its predecessor.
const KindTrustedAssertion = 30382

// TagRank is the NIP-85 rank tag key carrying the numeric score.
const TagRank = "rank"

// KindRegistryService is the addressable kind of a registr service record.
// Its d tag is the service name, so one coordinate names one service and a
// re-registration replaces its predecessor.
const KindRegistryService = 31339

// KindRegistryLink is the addressable kind of a registr link record. Its d tag
// is the link title.
const KindRegistryLink = 31340

// Job microstandard kinds (btk/jobs): one request kind, one result kind and
// one feedback kind for every job type in the estate. The job type rides as a
// NIP-32 label rather than a kind number, so no further kinds are allocated
// when a type is added. All three are regular events, stored by relays.
const (
	KindJobRequest  = 7710
	KindJobResult   = 7711
	KindJobFeedback = 7712
)

// JobLabelNamespace is the NIP-32 L value under which job types are labelled;
// every l tag naming a type carries it as its mark.
const JobLabelNamespace = "systems.buildtall.job"

// Tag names of the job microstandard: the NIP-32 label pair and the NIP-90
// request, result and feedback grammar it borrows.
const (
	TagLabelNamespace = "L"
	TagLabel          = "l"
	TagInput          = "i"
	TagParam          = "param"
	TagOutput         = "output"
	TagRequest        = "request"
	TagStatus         = "status"
	// TagOut is the estate's mirror of i on a result: one tag per output.
	TagOut = "out"
)

// nsite kinds (NIP-5A). A root site is replaceable, one per key, with no d
// tag; a named site is addressable under a d tag; a snapshot is a regular
// event pinning one version of either, addressed by its own id.
const (
	KindNsiteRoot     = 15128
	KindNsiteNamed    = 35128
	KindNsiteSnapshot = 5128
	// KindSiteSet is the site set of nuds/site-set.md: the named sites a
	// gateway serves, one a tag per kind 35128 coordinate.
	KindSiteSet = 30103
)

// Provenance kinds (nuds/provenance.md). A record states which article led
// its author to a feed, under the NIP-32 label namespace below; a provenance
// set is the leaf list that holds records.
const (
	KindProvenance           = 31985
	KindProvenanceSet        = 30104
	ProvenanceLabelNamespace = "systems.buildtall.provenance"
	ProvenanceLabel          = "provenance"
)

// KindVoice is the Choir voice of nuds/choir-voice.md: a public, addressable
// speaker a text-to-speech renderer can imitate, keyed by its slug d tag.
const KindVoice = 37703

// Tag names of an nsite manifest (NIP-5A). A path tag maps a site-absolute
// path to a blob hash; the x tag carries the aggregate hash of every path
// tag, marked by AggregateMarker in its third element; A names the origin
// site of a copy lineage and is copied unchanged onto a snapshot.
const (
	TagPath         = "path"
	TagAggregate    = "x"
	TagSource       = "source"
	TagDescription  = "description"
	TagOrigin       = "A"
	AggregateMarker = "aggregate"
)
