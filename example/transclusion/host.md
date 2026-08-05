# Transclusion

Every embed form in one note. Serve this directory and open this file:

```
stigmergic serve example/transclusion
```

## Whole note

![[sections]]

## One section

The heading, its text, and its subsections, ending at the next heading of the
same or a higher level. The same section is also embedded from
[[second-host]], so an edit to [[sections]] refreshes both pages.

![[sections#Habitats]]

## Section whose heading contains a wikilink

The fragment is the heading's text with the link brackets stripped:

![[sections#Naming host]]

## Image

![[glyph.png|a one-pixel glyph]]

## Attachment

![[notes.txt]]

## Inline

An embed with text beside it, ![[sections#Habitats]] for instance, stays in
the inline stream and renders as a link rather than transcluding.

## Cycle

The pair below embed each other; the repeat renders a marker instead of
recursing forever:

![[ouroboros-a]]

## Unresolved

A dangling target and an unmatched section are ordinary outcomes, not errors.
Each renders a visible marker:

![[no such note]]

![[sections#No Such Heading]]
