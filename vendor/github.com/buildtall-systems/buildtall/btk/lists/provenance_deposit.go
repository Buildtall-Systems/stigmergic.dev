package lists

import (
	"errors"
	"fmt"
	"time"

	"github.com/nbd-wtf/go-nostr"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
	"github.com/buildtall-systems/buildtall/btk/provenance"
)

// ProvenanceAct is one provenance act (nuds/provenance.md): the record the
// operator stated, the record already on the relay for the same subject,
// and the list the subject feed is followed into.
type ProvenanceAct struct {
	Current        *nostr.Event
	Record         provenance.Record
	UserNpub       string
	SubjectNpub    string
	FollowTarget   string
	FeedRelayHint  string
	ListsRelayHint string
	FollowDomain   Domain
}

// BuildProvenanceEvents produces the unsigned events of one provenance act,
// in publish order: the record, then its deposit in the vault-provenance
// root set, then the follow of the subject feed. Each part is omitted when
// it would change nothing, so an act repeated unchanged yields no events.
func BuildProvenanceEvents(act ProvenanceAct, existing []*nostr.Event, now time.Time) ([]*nostr.Event, error) {
	userHex, err := btknostr.NpubToHex(act.UserNpub)
	if err != nil {
		return nil, fmt.Errorf("converting user npub: %w", err)
	}
	if subjectHex, hexErr := btknostr.NpubToHex(act.SubjectNpub); hexErr != nil || subjectHex != act.Record.Subject {
		return nil, fmt.Errorf("subject npub %s does not name the record's subject", act.SubjectNpub)
	}

	var events []*nostr.Event
	record, err := recordEvent(act, now)
	if err != nil {
		return nil, err
	}
	if record != nil {
		events = append(events, record)
	}

	vaultEvents, err := provenanceVaultDeposit(act, existing, userHex)
	if err != nil {
		return nil, err
	}
	events = append(events, vaultEvents...)

	followEvents, err := BuildFollowEvents(act.FollowDomain, existing, act.UserNpub, act.SubjectNpub, act.FeedRelayHint, act.ListsRelayHint, act.FollowTarget)
	switch {
	case errors.Is(err, ErrItemAlreadyPresent):
	case err != nil:
		return nil, fmt.Errorf("following the subject feed: %w", err)
	default:
		events = append(events, followEvents...)
	}
	return events, nil
}

// recordEvent builds the record, or nil when the current record states the
// same via and note. A current record that does not parse is replaced.
func recordEvent(act ProvenanceAct, now time.Time) (*nostr.Event, error) {
	createdAt := nostr.Timestamp(now.Unix())
	if act.Current != nil {
		if current, err := provenance.Parse(act.Current); err == nil && act.Record.Same(current) {
			return nil, nil
		}
		createdAt = NextCreatedAt(act.Current, now)
	}
	ev, err := act.Record.Event(createdAt, act.ListsRelayHint)
	if err != nil {
		return nil, fmt.Errorf("building the provenance record: %w", err)
	}
	return ev, nil
}

// provenanceVaultDeposit lands the record's coordinate in the root set of
// the vault-provenance instance, minting the set and the vault root on
// first use.
func provenanceVaultDeposit(act ProvenanceAct, existing []*nostr.Event, userHex string) ([]*nostr.Event, error) {
	domain, err := VaultDomain(ProvenanceVault)
	if err != nil {
		return nil, fmt.Errorf("provenance vault: %w", err)
	}
	member := Item{
		Type:       "a",
		Value:      act.Record.Coordinate(userHex),
		RelayHint:  act.ListsRelayHint,
		SourceKind: btknostr.KindProvenance,
	}
	rootCoord := FormatCoordinate(KindListSet, userHex, domain.RootDTag)
	events, err := companionDeposit(domain, existing, act.UserNpub, userHex, domain.RootDTag, member, act.ListsRelayHint, rootCoord)
	if errors.Is(err, ErrItemAlreadyPresent) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("depositing the record in the provenance vault: %w", err)
	}
	return events, nil
}
