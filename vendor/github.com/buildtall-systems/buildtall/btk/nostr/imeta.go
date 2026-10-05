package nostr

import (
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

// TagIMeta is the NIP-92 media metadata tag. Each element after the name is
// one "key value" field, separated by the first space.
const TagIMeta = "imeta"

// imeta field keys this package reads (NIP-92, with NIP-94 field names).
const (
	imetaKeyURL      = "url"
	imetaKeyMime     = "m"
	imetaKeyFallback = "fallback"

	audioMimePrefix = "audio/"
)

// AudioRef is the audio an event carries: the primary URL, the alternate URLs
// that serve the same file in the order the tag lists them, and the MIME type.
type AudioRef struct {
	URL       string   `json:"url"`
	Mime      string   `json:"mime"`
	Fallbacks []string `json:"fallbacks,omitempty"`
}

// ParseAudio returns the first imeta tag whose MIME type is audio and which
// names a URL, or nil when there is none. Its fallbacks keep tag order and
// drop blanks, repeats, and the URL itself. r2n writes the tag in this shape.
func ParseAudio(tags nostr.Tags) *AudioRef {
	for _, tag := range tags {
		if len(tag) < 2 || tag[0] != TagIMeta {
			continue
		}
		var ref AudioRef
		var candidates []string
		for _, field := range tag[1:] {
			key, value, ok := strings.Cut(field, " ")
			if !ok {
				continue
			}
			switch key {
			case imetaKeyURL:
				ref.URL = value
			case imetaKeyMime:
				ref.Mime = value
			case imetaKeyFallback:
				candidates = append(candidates, value)
			}
		}
		if ref.URL == "" || !strings.HasPrefix(ref.Mime, audioMimePrefix) {
			continue
		}
		ref.Fallbacks = distinctFallbacks(ref.URL, candidates)
		return &ref
	}
	return nil
}

func distinctFallbacks(url string, candidates []string) []string {
	seen := map[string]bool{url: true}
	var fallbacks []string
	for _, candidate := range candidates {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		fallbacks = append(fallbacks, candidate)
	}
	return fallbacks
}
