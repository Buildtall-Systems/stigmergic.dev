package okf

import "github.com/buildtall-systems/buildtall/btk/lists"

// TypeDocument is the ratified OKF concept type for a kind 30023 long-form
// note, and the only type buildtall emits. The taxonomy grows by ratification
// in doctrine/07-vocabulary.md, never by a writer's invention.
//
// Under projection D no on-disk artifact carries a composition type: a
// directory is a kind 30004 curation set with no frontmatter of its own, and a
// vault's kind 30101 root is not a file at all. Concepts are documents and
// nothing else. Producer-defined types are still tolerated on read, since OKF
// requires consumers to accept types they do not know.
const TypeDocument = "Document"

var typeToKind = map[string]int{
	TypeDocument: lists.KindLongFormNote,
}

// TypeToKind maps an OKF concept type to a Nostr event kind. Unrecognized types
// map to the long-form note kind, honoring OKF's requirement that consumers
// tolerate unknown types; known reports whether the type was recognized.
func TypeToKind(conceptType string) (kind int, known bool) {
	if k, ok := typeToKind[conceptType]; ok {
		return k, true
	}
	return lists.KindLongFormNote, false
}

// KindToType maps a Nostr event kind to its canonical OKF type name. known
// reports whether the kind has a canonical mapping.
func KindToType(kind int) (conceptType string, known bool) {
	if kind == lists.KindLongFormNote {
		return TypeDocument, true
	}
	return "", false
}
