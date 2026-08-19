package lists

import (
	"errors"
	"fmt"
	"time"

	"github.com/nbd-wtf/go-nostr"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

// This file implements the read side of the ratified read/write preference
// concept (operations repo, concepts/read-and-write-lists.md): one kind
// 30078 event through which an owner designates the list they read and the
// list their deposits land in. drss, btcli, and Android are peer readers of
// this one parse.

const (
	// KindApplicationData is NIP-78 application-specific data, the kind the
	// preference event uses. It is not a list kind: the preference lives
	// outside the ontology by the composition rule, so it does not join
	// ListKinds or the composition table.
	KindApplicationData = 30078
	// DTagReadWriteLists is the ratified d-tag of the preference event. No
	// application prefix: the preference names the owner's posture across
	// applications, so no application owns its name.
	DTagReadWriteLists = "read-write-lists"

	roleMarkerRead  = "read"
	roleMarkerWrite = "write"
)

// ListRoles carries the two designated list coordinates. An empty role means
// the event designated nothing valid for it and the reader falls back to the
// domain's zero-config default (CanonicalRootCoord).
type ListRoles struct {
	Read  string
	Write string
}

// ParseRolePreference reads a read/write preference event per the ratified
// concept doc. Validity is per role: each unmarked reference counts toward
// both roles; a role with exactly one reference resolves to it; zero or
// surplus references leave the role empty, which is the fall-back signal. A
// reference is discarded for both roles when its coordinate does not parse,
// names a kind outside 30101 and the NIP-51 set kinds, or names a pubkey
// other than the signer. A nil event, a wrong kind, or a wrong d-tag yields
// the zero value: both roles fall back.
func ParseRolePreference(ev *nostr.Event) ListRoles {
	if ev == nil || ev.Kind != KindApplicationData || GetDTag(ev) != DTagReadWriteLists {
		return ListRoles{}
	}

	var reads, writes []string
	for _, tag := range ev.Tags {
		if len(tag) < 2 || tag[0] != "a" {
			continue
		}
		kind, pubkeyHex, _, err := btknostr.ParseCoordinate(tag[1])
		if err != nil || pubkeyHex != ev.PubKey || !AllowedCompositionKind(kind) {
			continue
		}
		switch roleMarker(tag) {
		case roleMarkerRead:
			reads = append(reads, tag[1])
		case roleMarkerWrite:
			writes = append(writes, tag[1])
		default:
			reads = append(reads, tag[1])
			writes = append(writes, tag[1])
		}
	}

	var roles ListRoles
	if len(reads) == 1 {
		roles.Read = reads[0]
	}
	if len(writes) == 1 {
		roles.Write = writes[0]
	}
	return roles
}

// NewRolePreferenceEvent builds the unsigned read/write preference event
// designating one read list and one write list, per the ratified concept
// doc. Equal designations emit one unmarked reference, which serves both
// roles. The owner's npub decodes to hex here — the protocol boundary
// minting the event — and the result passes ValidatePreference before it is
// returned, so a caller never holds an event readers would fall back from.
func NewRolePreferenceEvent(ownerNpub, readCoord, writeCoord, relayHint string) (*nostr.Event, error) {
	ownerHex, err := btknostr.NpubToHex(ownerNpub)
	if err != nil {
		return nil, fmt.Errorf("owner npub: %w", err)
	}

	tags := nostr.Tags{{"d", DTagReadWriteLists}}
	if readCoord == writeCoord {
		tags = append(tags, roleTag(readCoord, relayHint, ""))
	} else {
		tags = append(tags,
			roleTag(readCoord, relayHint, roleMarkerRead),
			roleTag(writeCoord, relayHint, roleMarkerWrite))
	}

	ev := &nostr.Event{
		Kind:      KindApplicationData,
		PubKey:    ownerHex,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Tags:      tags,
		Content:   "",
	}
	if err := ValidatePreference(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// roleTag renders one reference in the ratified shape: coordinate, relay
// hint third, role marker fourth. An unmarked reference omits the marker; a
// marker with no hint keeps the empty third position so the marker stays in
// the NIP-65 grammar's reach.
func roleTag(coord, relayHint, marker string) nostr.Tag {
	tag := nostr.Tag{"a", coord}
	if relayHint != "" || marker != "" {
		tag = append(tag, relayHint)
	}
	if marker != "" {
		tag = append(tag, marker)
	}
	return tag
}

// ValidatePreference rejects a preference event a reader would discard or
// fall back from, for writers that must not publish what readers ignore:
// wrong kind, wrong d-tag, an unparseable reference, a reference authored by
// someone other than the event signer, a reference outside the allowed list
// kinds, or surplus references for a role (an unmarked reference counts
// toward both). Parsing stays lenient in ParseRolePreference; this is the
// strict door a publishing ceremony puts in front of it.
func ValidatePreference(ev *nostr.Event) error {
	if ev == nil {
		return errors.New("no event")
	}
	if ev.Kind != KindApplicationData {
		return fmt.Errorf("wrong kind %d: the preference event is kind %d", ev.Kind, KindApplicationData)
	}
	if dTag := GetDTag(ev); dTag != DTagReadWriteLists {
		return fmt.Errorf("wrong d-tag %q: the ratified d-tag is %q", dTag, DTagReadWriteLists)
	}

	reads, writes := 0, 0
	for _, tag := range ev.Tags {
		if len(tag) < 2 || tag[0] != "a" {
			continue
		}
		kind, pubkeyHex, _, err := btknostr.ParseCoordinate(tag[1])
		if err != nil {
			return fmt.Errorf("reference %q: %w", tag[1], err)
		}
		if pubkeyHex != ev.PubKey {
			return fmt.Errorf("reference %s is not authored by the event signer", tag[1])
		}
		if !AllowedCompositionKind(kind) {
			return fmt.Errorf("reference %s names kind %d, which cannot serve a role", tag[1], kind)
		}
		switch roleMarker(tag) {
		case roleMarkerRead:
			reads++
		case roleMarkerWrite:
			writes++
		default:
			reads++
			writes++
		}
	}
	if reads > 1 {
		return fmt.Errorf("%d read references: a role takes exactly one", reads)
	}
	if writes > 1 {
		return fmt.Errorf("%d write references: a role takes exactly one", writes)
	}
	return nil
}

// roleMarker finds the reference's role marker past the coordinate: the
// relay hint in the third position is optional, so the marker is the first
// recognized value rather than a fixed index (the NIP-65 grammar the doc
// borrows). Anything unrecognized reads as unmarked.
func roleMarker(tag nostr.Tag) string {
	for _, v := range tag[2:] {
		if v == roleMarkerRead || v == roleMarkerWrite {
			return v
		}
	}
	return ""
}
