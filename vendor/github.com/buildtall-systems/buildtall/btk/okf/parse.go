package okf

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const frontmatterFence = "---"

// ParseConcept parses an OKF concept document. conceptID is the concept's
// bundle-relative path without the ".md" suffix, used only for error context
// and to populate Concept.ConceptID. It returns an error if the document lacks
// a frontmatter block or if the required "type" field is empty.
func ParseConcept(data []byte, conceptID string) (*Concept, error) {
	fm, body, found, err := SplitFrontmatter(data)
	if err != nil {
		return nil, fmt.Errorf("okf: concept %q: %w", conceptID, err)
	}
	if !found {
		return nil, fmt.Errorf("okf: concept %q: missing YAML frontmatter (document must begin with %q)", conceptID, frontmatterFence)
	}

	var front Frontmatter
	if err := yaml.Unmarshal(fm, &front); err != nil {
		return nil, fmt.Errorf("okf: concept %q: parsing frontmatter: %w", conceptID, err)
	}
	if strings.TrimSpace(front.Type) == "" {
		return nil, fmt.Errorf("okf: concept %q: missing required %q field", conceptID, FieldType)
	}
	if err := front.CheckNostrTags(); err != nil {
		return nil, fmt.Errorf("okf: concept %q: %w", conceptID, err)
	}

	return &Concept{
		ConceptID:   conceptID,
		Frontmatter: front,
		Body:        body,
	}, nil
}

// SplitFrontmatter separates the leading YAML frontmatter block from the
// markdown body. A frontmatter block begins with a "---" fence line and closes
// with another "---" fence line; exactly one blank separator line after the
// closing fence is consumed so that serialization re-adds it idempotently.
//
// A document that does not begin with the fence carries no frontmatter: found
// is false and the whole document is the body, which is an answer rather than
// an error because a lenient reader synthesizes over it while the strict one
// refuses it. A document that opens the fence and never closes it is an error
// in any reading. This is the one statement of the fence rules; every reader
// splits here.
func SplitFrontmatter(data []byte) (frontmatter []byte, body string, found bool, err error) {
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != frontmatterFence {
		return nil, string(data), false, nil
	}

	closing := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == frontmatterFence {
			closing = i
			break
		}
	}
	if closing == -1 {
		return nil, "", false, fmt.Errorf("unterminated frontmatter block (no closing %q)", frontmatterFence)
	}

	frontmatter = []byte(strings.Join(lines[1:closing], "\n"))
	body = strings.Join(lines[closing+1:], "\n")
	body = strings.TrimPrefix(body, "\n")
	return frontmatter, body, true, nil
}
