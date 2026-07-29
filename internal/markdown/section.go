package markdown

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/frontmatter"
	"go.abhg.dev/goldmark/wikilink"

	"github.com/Buildtall-Systems/stigmergic.dev/internal/models"
)

// sectionHeading records one heading's position and flattened text, which is
// everything section slicing needs after the parse is discarded.
type sectionHeading struct {
	text  string
	level int
	start int
}

// ExtractSection returns the bytes of the section introduced by the heading
// whose text equals fragment, together with a flag reporting whether such a
// heading was found.
//
// The section begins at the heading's own line, so the ATX marker or setext
// underline survives into the slice and the section renders as a heading
// again. It ends where the next heading of the same or a higher level begins,
// which means descendant subsections are included by construction.
//
// Matching is exact and case-sensitive across the whole document first; only
// when that finds nothing does a case-insensitive pass run. There is no prefix
// or substring fallback, because a document may carry both "floodlights" and
// "on the floodlights" as sibling headings and a fragment naming the former
// must not select the latter.
func ExtractSection(source []byte, fragment string) ([]byte, bool) {
	want := strings.TrimSpace(fragment)
	if want == "" {
		return nil, false
	}

	headings := collectSectionHeadings(source)
	if len(headings) == 0 {
		return nil, false
	}

	match := -1
	for i, h := range headings {
		if h.text == want {
			match = i
			break
		}
	}
	if match < 0 {
		for i, h := range headings {
			if strings.EqualFold(h.text, want) {
				match = i
				break
			}
		}
	}
	if match < 0 {
		return nil, false
	}

	end := len(source)
	for _, h := range headings[match+1:] {
		if h.level <= headings[match].level {
			end = h.start
			break
		}
	}

	return source[headings[match].start:end], true
}

// collectSectionHeadings parses source for headings alone and returns them in
// document order.
//
// The parser mirrors ExtractOutline's configuration, with one addition that is
// load-bearing: the wikilink inline parser is installed at priority 199, the
// same priority Parse uses. Without it a heading such as
// "## [[03-02-23]]" flattens to its literal brackets rather than to the link
// text, and a fragment naming the heading as it reads on screen never matches.
func collectSectionHeadings(source []byte) []sectionHeading {
	md := goldmark.New(
		goldmark.WithExtensions(&frontmatter.Extender{}),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithInlineParsers(util.Prioritized(&wikilink.Parser{}, 199)),
		),
	)

	doc := md.Parser().Parse(text.NewReader(source))

	var headings []sectionHeading
	if err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		if heading.Lines().Len() == 0 {
			return ast.WalkContinue, nil
		}
		headings = append(headings, sectionHeading{
			level: heading.Level,
			text:  strings.TrimSpace(string(appendInlineText(nil, heading, source))),
			start: lineStart(source, heading.Lines().At(0).Start),
		})
		return ast.WalkContinue, nil
	}); err != nil {
		return nil
	}
	return headings
}

// lineStart returns the offset of the first byte on the line containing
// offset. goldmark reports a heading's content position, which for an ATX
// heading sits after the marker, so the marker is recovered by scanning back.
func lineStart(source []byte, offset int) int {
	if offset > len(source) {
		offset = len(source)
	}
	if idx := bytes.LastIndexByte(source[:offset], '\n'); idx >= 0 {
		return idx + 1
	}
	return 0
}

// EmbedTargetKind classifies an embed target by its file extension, which is
// the only signal available before anything is resolved.
type EmbedTargetKind int

const (
	// EmbedTargetNote is a markdown target, transcluded as content.
	EmbedTargetNote EmbedTargetKind = iota
	// EmbedTargetImage is an image target, rendered as an image element.
	EmbedTargetImage
	// EmbedTargetAttachment is any other file, rendered as an anchor.
	EmbedTargetAttachment
)

// imageExts is the set of extensions rendered as images, copied from the
// wikilink package's own resolveAsImage list, which in turn follows MDN's
// image type table.
var imageExts = map[string]struct{}{
	".apng": {}, ".avif": {}, ".gif": {}, ".jpg": {}, ".jpeg": {},
	".jfif": {}, ".pjpeg": {}, ".pjp": {}, ".png": {}, ".svg": {}, ".webp": {},
}

// ClassifyEmbedTarget reports how an embed target should be rendered. A target
// without an extension is a note, matching the vault convention of writing
// "[[some note]]" rather than "[[some note.md]]". Extensions are compared
// case-insensitively, since a vault may hold "IMG_0001.PNG".
//
// A trailing dotted run containing a space is not an extension. Note titles
// such as "v1.2 release" and "Ch. 3 notes" are ordinary in a vault, and
// filepath.Ext reports ".2 release" and ". 3 notes" for them; treating those
// as attachments would send a perfectly resolvable note to the filesystem
// probe and render a marker instead of its content.
func ClassifyEmbedTarget(target string) EmbedTargetKind {
	ext := strings.ToLower(filepath.Ext(target))
	if ext == "" || ext == models.MarkdownExt || strings.ContainsAny(ext, " \t") {
		return EmbedTargetNote
	}
	if _, ok := imageExts[ext]; ok {
		return EmbedTargetImage
	}
	return EmbedTargetAttachment
}
