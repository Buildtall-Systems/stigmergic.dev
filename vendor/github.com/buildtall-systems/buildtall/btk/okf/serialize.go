package okf

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// WriteConcept serializes a Concept back to OKF document bytes: a "---"-fenced
// YAML frontmatter block followed by the markdown body. Frontmatter keys are
// emitted in canonical order (the known keys first, then producer-defined Extra
// keys sorted), so the output is deterministic and idempotent: re-parsing and
// re-serializing yields byte-identical output.
func WriteConcept(c *Concept) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("okf: cannot serialize nil concept")
	}
	if strings.TrimSpace(c.Frontmatter.Type) == "" {
		return nil, fmt.Errorf("okf: concept %q: missing required %q field", c.ConceptID, FieldType)
	}

	fm, err := marshalFrontmatter(c.Frontmatter)
	if err != nil {
		return nil, fmt.Errorf("okf: concept %q: marshaling frontmatter: %w", c.ConceptID, err)
	}

	var b strings.Builder
	b.WriteString(frontmatterFence)
	b.WriteString("\n")
	b.Write(fm)
	if !strings.HasSuffix(string(fm), "\n") {
		b.WriteString("\n")
	}
	b.WriteString(frontmatterFence)
	b.WriteString("\n")
	if c.Body != "" {
		b.WriteString("\n")
		b.WriteString(c.Body)
	}
	if !strings.HasSuffix(b.String(), "\n") {
		b.WriteString("\n")
	}
	return []byte(b.String()), nil
}

// marshalFrontmatter builds a mapping node in canonical key order and marshals
// it. Empty recommended fields are omitted; Extra keys are appended in sorted
// order to keep the output deterministic.
func marshalFrontmatter(f Frontmatter) ([]byte, error) {
	m := &yaml.Node{Kind: yaml.MappingNode}

	appendScalar := func(key, val string) {
		if val == "" {
			return
		}
		m.Content = append(m.Content, strScalar(key), strScalar(val))
	}

	appendScalar(FieldType, f.Type)
	appendScalar(FieldTitle, f.Title)
	appendScalar(FieldDescription, f.Description)
	appendScalar(FieldResource, f.Resource)
	if len(f.Tags) > 0 {
		seq := &yaml.Node{Kind: yaml.SequenceNode}
		for _, t := range f.Tags {
			seq.Content = append(seq.Content, strScalar(t))
		}
		m.Content = append(m.Content, strScalar(FieldTags), seq)
	}
	appendScalar(FieldTimestamp, f.Timestamp)
	appendScalar(FieldDTag, f.DTag)
	if len(f.NostrTags) > 0 {
		m.Content = append(m.Content, strScalar(NostrTagsKey), nostrTagsNode(f.NostrTags))
	}

	if err := appendExtra(m, f.Extra); err != nil {
		return nil, err
	}

	return yaml.Marshal(m)
}

// appendExtra states the producer-defined keys onto a mapping node in sorted
// order. The file's frontmatter block and the okf-extra wire tag both build
// their carriage here, which is what makes the two spellings agree by
// construction; a second encoder would be a quiet-republish break waiting for
// a divergence.
func appendExtra(m *yaml.Node, extra map[string]any) error {
	for _, key := range sortedKeys(extra) {
		valNode := &yaml.Node{}
		if err := valNode.Encode(extra[key]); err != nil {
			return fmt.Errorf("encoding extra key %q: %w", key, err)
		}
		m.Content = append(m.Content, strScalar(key), valNode)
	}
	return nil
}

// marshalExtra renders the producer-defined keys alone as one canonical YAML
// document: the okf-extra tag's value.
func marshalExtra(extra map[string]any) (string, error) {
	m := &yaml.Node{Kind: yaml.MappingNode}
	if err := appendExtra(m, extra); err != nil {
		return "", err
	}
	out, err := yaml.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// unmarshalExtra is marshalExtra's inverse, applying the same decode every
// frontmatter read applies, so a map that has been through the tag equals one
// that has been through a file. A value that decodes to no keys yields nil: an
// emptiness the emitter never writes, read as the absence it states.
func unmarshalExtra(value string) (map[string]any, error) {
	extra := map[string]any{}
	if err := yaml.Unmarshal([]byte(value), &extra); err != nil {
		return nil, err
	}
	if len(extra) == 0 {
		return nil, nil
	}
	return extra, nil
}

// nostrTagsNode renders a raw tag list as a sequence of flow sequences. Flow
// style keeps one tag on one line, which is how a tag reads on the wire and
// what makes a hand edit obvious in a diff. A concept states its tags in
// frontmatter and a directory states them in its sidecar, so both build the
// carrier here rather than each spelling it out.
func nostrTagsNode(tags [][]string) *yaml.Node {
	outer := &yaml.Node{Kind: yaml.SequenceNode}
	for _, tag := range tags {
		inner := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
		for _, element := range tag {
			inner.Content = append(inner.Content, strScalar(element))
		}
		outer.Content = append(outer.Content, inner)
	}
	return outer
}

// strScalar builds a string-tagged scalar node so values that would otherwise
// parse as non-strings (timestamps, numbers) are quoted and round-trip as text.
func strScalar(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

// intScalar builds an integer-tagged scalar so a version reads as the number it
// is rather than as a quoted string, which is what lets a hand edit that drops
// the quotes still parse.
func intScalar(n int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(n)}
}

func sortedKeys(m map[string]any) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
