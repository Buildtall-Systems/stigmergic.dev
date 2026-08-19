// Package okf implements the Open Knowledge Format (OKF) as buildtall's
// on-disk representation of a region of the personal ontology.
//
// OKF is the file/git-native dual of the ontology: a directory tree of
// markdown files with YAML frontmatter. Under projection D the tree's shape
// lives in names rather than in edges. A directory is a kind 30004 curation
// set whose d-tag is that directory's verbatim path, a concept file is a kind
// 30023 document one of those sets references, and a single kind 30101 root
// references every set in the bundle. Nesting is recovered lexically from the
// paths, so no traversal is needed to learn the shape. The format is
// deliberately minimal and its consumption model is permissive: the only
// conformance requirement is that every concept carry a non-empty "type"
// field.
//
// A vault may name where its attachment bytes live: the root event carries
// one or more okf-server tags, each valued with a Blossom base URL, in
// preference order. The tag is stated in the root sidecar's nostr tags and
// round-trips like any sidecar tag; a reader that finds none falls back to
// the owner's kind 10063 server list.
//
// This package provides the concept-level primitives: parsing a concept
// document into a Concept, and serializing a Concept back to bytes such that
// the operation is lossless in content and idempotent in form. Unknown
// producer-defined frontmatter keys are preserved across the round trip.
//
// The rationale and design decisions are recorded in
// docs/architecture/decisions/007-okf-as-on-disk-format.md.
package okf
