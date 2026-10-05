// Package provenance implements the kind 31985 provenance record of
// nuds/provenance.md: which article led its author to a feed, and why.
package provenance

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/nbd-wtf/go-nostr"

	btknostr "github.com/buildtall-systems/buildtall/btk/nostr"
)

const (
	tagD         = "d"
	tagP         = "p"
	tagA         = "a"
	tagI         = "i"
	tagK         = "k"
	tagNamespace = "L"
	tagLabel     = "l"
	externalWeb  = "web"
)

// Record is one provenance record. Subject and the pubkey inside Via are hex,
// the wire form the event carries.
type Record struct {
	Subject    string
	Via        string
	Note       string
	SubjectURL string
}

// ReferringFeed is the pubkey of the feed that published the via.
func (r Record) ReferringFeed() string {
	_, pubkey, _, err := btknostr.ParseCoordinate(r.Via)
	if err != nil {
		return ""
	}
	return pubkey
}

// Coordinate is the record's address under its author.
func (r Record) Coordinate(authorHex string) string {
	return fmt.Sprintf("%d:%s:%s", btknostr.KindProvenance, authorHex, r.Subject)
}

// Same reports whether a republish of r over current would change nothing
// the author stated: the via and the note.
func (r Record) Same(current Record) bool {
	return r.Via == current.Via && strings.TrimSpace(r.Note) == strings.TrimSpace(current.Note)
}

// Validate applies the Validity rules of nuds/provenance.md.
func (r Record) Validate() error {
	if !nostr.IsValid32ByteHex(r.Subject) {
		return fmt.Errorf("subject %q is not 64 lowercase hex characters", r.Subject)
	}
	kind, pubkey, dTag, err := btknostr.ParseCoordinate(r.Via)
	if err != nil {
		return fmt.Errorf("via: %w", err)
	}
	if kind != btknostr.KindLongForm {
		return fmt.Errorf("via names kind %d; it must name a kind %d article", kind, btknostr.KindLongForm)
	}
	if !nostr.IsValid32ByteHex(pubkey) {
		return fmt.Errorf("via pubkey %q is not 64 lowercase hex characters", pubkey)
	}
	if dTag == "" {
		return errors.New("via names an article without a d tag")
	}
	if pubkey == r.Subject {
		return errors.New("a feed does not lead to itself: the via is the subject's own article")
	}
	if strings.TrimSpace(r.Note) == "" {
		return errors.New("the note is required")
	}
	if r.SubjectURL != "" {
		if err := validateSubjectURL(r.SubjectURL); err != nil {
			return err
		}
	}
	return nil
}

func validateSubjectURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("subject url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("subject url %q is not http or https", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("subject url %q has no host", raw)
	}
	if strings.Contains(raw, "#") {
		return fmt.Errorf("subject url %q carries a fragment", raw)
	}
	return nil
}

// Event builds the unsigned record. The relay hint rides the p and a tags;
// the note is written without leading or trailing white space.
func (r Record) Event(createdAt nostr.Timestamp, relayHint string) (*nostr.Event, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	tags := nostr.Tags{
		{tagD, r.Subject},
		{tagNamespace, btknostr.ProvenanceLabelNamespace},
		{tagLabel, btknostr.ProvenanceLabel, btknostr.ProvenanceLabelNamespace},
		{tagP, r.Subject, relayHint},
		{tagA, r.Via, relayHint},
	}
	if r.SubjectURL != "" {
		tags = append(tags, nostr.Tag{tagI, r.SubjectURL}, nostr.Tag{tagK, externalWeb})
	}
	return &nostr.Event{
		Kind:      btknostr.KindProvenance,
		CreatedAt: createdAt,
		Tags:      tags,
		Content:   strings.TrimSpace(r.Note),
	}, nil
}

// Parse reads a record from an event and validates it. A reader ignores a
// record Parse refuses.
func Parse(ev *nostr.Event) (Record, error) {
	if ev == nil {
		return Record{}, errors.New("event is nil")
	}
	if ev.Kind != btknostr.KindProvenance {
		return Record{}, fmt.Errorf("kind %d is not a provenance record", ev.Kind)
	}
	d, err := only(ev, tagD)
	if err != nil {
		return Record{}, err
	}
	p, err := only(ev, tagP)
	if err != nil {
		return Record{}, err
	}
	if d != p {
		return Record{}, fmt.Errorf("d tag %q differs from p tag %q", d, p)
	}
	via, err := only(ev, tagA)
	if err != nil {
		return Record{}, err
	}
	if !hasNamespace(ev) {
		return Record{}, fmt.Errorf("missing L %s and l %s", btknostr.ProvenanceLabelNamespace, btknostr.ProvenanceLabel)
	}
	subjectURL, err := externalURL(ev)
	if err != nil {
		return Record{}, err
	}
	r := Record{Subject: d, Via: via, Note: ev.Content, SubjectURL: subjectURL}
	if err := r.Validate(); err != nil {
		return Record{}, err
	}
	return r, nil
}

func only(ev *nostr.Event, name string) (string, error) {
	var values []string
	for _, tag := range ev.Tags {
		if len(tag) >= 2 && tag[0] == name {
			values = append(values, tag[1])
		}
	}
	if len(values) != 1 {
		return "", fmt.Errorf("want exactly one %s tag, got %d", name, len(values))
	}
	return values[0], nil
}

func hasNamespace(ev *nostr.Event) bool {
	var namespace, label bool
	for _, tag := range ev.Tags {
		switch {
		case len(tag) >= 2 && tag[0] == tagNamespace && tag[1] == btknostr.ProvenanceLabelNamespace:
			namespace = true
		case len(tag) >= 3 && tag[0] == tagLabel && tag[1] == btknostr.ProvenanceLabel && tag[2] == btknostr.ProvenanceLabelNamespace:
			label = true
		}
	}
	return namespace && label
}

func externalURL(ev *nostr.Event) (string, error) {
	var ids, kinds []string
	for _, tag := range ev.Tags {
		if len(tag) < 2 {
			continue
		}
		switch tag[0] {
		case tagI:
			ids = append(ids, tag[1])
		case tagK:
			kinds = append(kinds, tag[1])
		}
	}
	switch {
	case len(ids) == 0:
		return "", nil
	case len(ids) > 1:
		return "", fmt.Errorf("want at most one i tag, got %d", len(ids))
	case len(kinds) != 1 || kinds[0] != externalWeb:
		return "", fmt.Errorf("an i tag needs one k tag %q", externalWeb)
	}
	return ids[0], nil
}
