package okf

import (
	"errors"
	"fmt"
	"strings"

	"github.com/buildtall-systems/buildtall/btk/lists"
)

// Identity under projection D: a concept's bundle path is its name on the
// wire. The path is carried into the d-tag verbatim, so no transformation
// stands between the file a person edits and the event it publishes as, and a
// bundle path can always be recovered from a d-tag exactly.

// DTagForConcept returns the addressable-event d-tag for a concept within the
// given domain: the explicit "d" frontmatter value when present, otherwise the
// domain's member d-tag for the concept's bundle path (ConceptID). An explicit
// d-tag wins because it carries the identity a concept already had as an
// event, which is what lets a renamed file republish under its original name.
func DTagForConcept(domain lists.Domain, c *Concept) (string, error) {
	if c == nil {
		return "", errors.New("okf: cannot derive d-tag from nil concept")
	}
	if d := strings.TrimSpace(c.Frontmatter.DTag); d != "" {
		if strings.Contains(d, ":") {
			return "", fmt.Errorf(`okf: concept %q: explicit d-tag %q contains ":", which delimits coordinate fields`, c.ConceptID, d)
		}
		return d, nil
	}
	dTag, err := domain.MemberDTag(c.ConceptID)
	if err != nil {
		return "", fmt.Errorf("okf: concept %q: %w", c.ConceptID, err)
	}
	if err := CheckPathSegments(c.ConceptID, domain.PathSeparator); err != nil {
		return "", fmt.Errorf("okf: concept %q: %w", c.ConceptID, err)
	}
	return dTag, nil
}

// MemberDTagForConcept resolves a concept's d-tag for the write direction,
// refusing any result that is not a member of the given domain. The permissive
// contract of DTagForConcept is right for reading an event back and wrong for
// emitting one: it validates an explicit "d" value against ":" alone, so a
// hand-written "d: drss-tech" resolves cleanly into another application's
// namespace, and nip-101 requires that a writer emit only d-tags belonging to
// the domain it declared. The domain's own root is refused for the same
// reason a concept is not the vault that holds it.
func MemberDTagForConcept(domain lists.Domain, c *Concept) (string, error) {
	dTag, err := DTagForConcept(domain, c)
	if err != nil {
		return "", err
	}
	if domain.ClassifyDTag(dTag) != lists.DTagMember {
		return "", fmt.Errorf("okf: concept %q: d-tag %q is not a member of domain %q, whose members carry the prefix %q", c.ConceptID, dTag, domain.Name, domain.DTagPrefix)
	}
	// An explicit d-tag reaches here without having passed the filesystem
	// guard, since DTagForConcept applies it only to a derived path.
	if err := CheckPathSegments(strings.TrimPrefix(dTag, domain.DTagPrefix), domain.PathSeparator); err != nil {
		return "", fmt.Errorf("okf: concept %q: d-tag %q: %w", c.ConceptID, dTag, err)
	}
	return dTag, nil
}

// CheckDTagMatchesPath refuses a concept whose stamped "d" frontmatter value
// disagrees with the d-tag its own bundle path derives. The export stamps an
// explicit d-tag on every concept it writes, so moving a file inside a bundle
// leaves the stamp naming where the file used to be. Publishing that stamp
// would republish the concept at its old path and the next export would move
// the file back, reverting a deliberate edit without saying so. Refusing and
// naming both paths puts the choice where it belongs.
func CheckDTagMatchesPath(domain lists.Domain, c *Concept) error {
	if c == nil {
		return errors.New("okf: cannot check the d-tag of a nil concept")
	}
	explicit := strings.TrimSpace(c.Frontmatter.DTag)
	if explicit == "" {
		return nil
	}
	derived, err := domain.MemberDTag(c.ConceptID)
	if err != nil {
		return fmt.Errorf("okf: concept %q: %w", c.ConceptID, err)
	}
	if explicit == derived {
		return nil
	}
	stamped := explicit
	if path, err := domain.MemberPath(explicit); err == nil {
		stamped = path
	}
	return fmt.Errorf("okf: concept at path %q carries a d-tag stamped from path %q: publishing would move it back. Remove or update the stamped d-tag to publish it where it now sits", c.ConceptID, stamped)
}

// DTagToPath recovers a concept's bundle-relative path from its d-tag,
// inverting the derived direction of DTagForConcept. It is how an exporter
// rebuilds a tree from the d-tags it fetched and how an importer rewrites
// links between concepts.
func DTagToPath(domain lists.Domain, dTag string) (string, error) {
	path, err := domain.MemberPath(dTag)
	if err != nil {
		return "", fmt.Errorf("okf: %w", err)
	}
	if err := CheckPathSegments(path, domain.PathSeparator); err != nil {
		return "", fmt.Errorf("okf: d-tag %q: %w", dTag, err)
	}
	return path, nil
}

// CheckPathSegments rejects paths a bundle cannot materialize faithfully. The
// ontology's own rules (a path must name something, may not carry ":", may not
// have empty segments) belong to btk/lists and are enforced there. What a
// filesystem adds is that "." and ".." navigate rather than name, so a path
// containing either would read or write outside the directory it claims to
// describe. The root set's reserved segment is refused outright: on disk
// nothing carries that name, since the reconciler mints the root set and the
// exporter maps its holdings back onto the bundle root, so a real file or
// directory named for it would counterfeit a derived event. Both directions of
// the bundle apply it, so that reading and writing refuse exactly the same
// paths.
func CheckPathSegments(path, separator string) error {
	if path == "" {
		return errors.New("path is empty")
	}
	for segment := range strings.SplitSeq(path, separator) {
		switch segment {
		case "":
			return fmt.Errorf("path %q has an empty segment", path)
		case ".", "..":
			return fmt.Errorf("path %q has segment %q, which navigates rather than names", path, segment)
		case lists.VaultRootSegment:
			return fmt.Errorf("path %q has segment %q, which is reserved for the root set", path, segment)
		}
	}
	return nil
}

// CheckDTags reports two concepts in one bundle that resolve to the same
// d-tag, which would make them a single addressable event under one keypair
// and destroy whichever the relay saw first. Every concept's resolved d-tag
// enters the check, derived as well as explicit. Verbatim path mapping is
// injective among derived d-tags, so those cannot collide with each other, but
// an explicit value is under no such constraint and can name the path another
// concept already occupies. Detecting that requires the domain, since a
// derived d-tag does not exist until a domain renders it.
func CheckDTags(domain lists.Domain, concepts []*Concept) error {
	seen := make(map[string]string, len(concepts))
	for _, c := range concepts {
		if c == nil {
			return errors.New("okf: nil concept in bundle")
		}
		d, err := DTagForConcept(domain, c)
		if err != nil {
			return err
		}
		if prev, ok := seen[d]; ok {
			return fmt.Errorf("okf: d-tag collision: concepts %q and %q both resolve to d-tag %q", prev, c.ConceptID, d)
		}
		seen[d] = c.ConceptID
	}
	return nil
}
