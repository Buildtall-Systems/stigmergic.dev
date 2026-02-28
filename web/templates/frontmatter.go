package templates

import (
	"fmt"
	"sort"
)

// MetadataValue classifies a frontmatter value for template rendering.
type MetadataValue struct {
	StringVal string
	ArrayVal  []string
	IsArray   bool
	IsHidden  bool // nested maps are omitted from display
}

// ClassifyMetadataValue converts an arbitrary frontmatter value into a
// type the template can render without Go type assertions.
func ClassifyMetadataValue(v any) MetadataValue {
	switch val := v.(type) {
	case string:
		return MetadataValue{StringVal: val}
	case []any:
		strs := make([]string, 0, len(val))
		for _, item := range val {
			strs = append(strs, fmt.Sprint(item))
		}
		return MetadataValue{ArrayVal: strs, IsArray: true}
	case []string:
		return MetadataValue{ArrayVal: val, IsArray: true}
	case map[string]any:
		return MetadataValue{IsHidden: true}
	case bool:
		if val {
			return MetadataValue{StringVal: "Yes"}
		}
		return MetadataValue{StringVal: "No"}
	default:
		return MetadataValue{StringVal: fmt.Sprint(val)}
	}
}

// SortedMetadataKeys returns deterministic key ordering: "title" first if
// present, then remaining keys alphabetically.
func SortedMetadataKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Promote "title" to first position.
	for i, k := range keys {
		if k == "title" {
			keys = append(keys[:i], keys[i+1:]...)
			keys = append([]string{"title"}, keys...)
			break
		}
	}

	return keys
}
