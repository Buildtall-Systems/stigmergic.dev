# stigmergic.dev UI Redesign — wiremd Mockups

Scope, per operator direction: persistent sidebar layout, document outline (ToC),
follow mode, full-text search in the command palette, refined header (live
indicator removed). View this file in stigmergic itself — the wiremd blocks
render as sketch wireframes.

A note on the sketches: wiremd grids render equal-width columns. The real
layout is asymmetric — a fixed ~260px sidebar, a fluid content column capped at
a comfortable reading measure (~72ch), and a fixed ~220px outline rail. Read
the grid columns as panes, not proportions.

---

## 1. Refined Header

The live indicator is removed. The right side of the header becomes a compact
control cluster: a visible search affordance (making Ctrl+K discoverable), the
follow-mode toggle, a theme toggle, and a keyboard-help trigger. The logo and
wordmark stay on the left and remain the link home.

```wiremd
::: navbar
## stigmergic
[Search  Ctrl+K]{.secondary}  [Follow: Off]{.secondary}  [Theme]{.secondary}  [?]{.secondary}
:::
```

Design notes:

- The search button opens the same command palette as Ctrl+K.
- "Follow" is a stateful toggle — see mockup 4 for its active state.
- "Theme" cycles iceberg-dark / iceberg-light (custom themes still config-driven).
- "?" opens the shortcuts overlay — see mockup 6.
- The scroll-progress bar stays; it is chrome-light and earns its pixel.

---

## 2. Reading View — Three-Pane Layout

The core structural change: navigation and reading are no longer mutually
exclusive. The file tree persists in a left sidebar with the current file
highlighted; the document occupies the center at a constrained reading measure;
a right rail carries the document outline with scrollspy. Both rails collapse
(sidebar via a header chevron, outline auto-hides under ~1100px viewport).

```wiremd
## Reading View {.grid-3}

### Files

- research/
- plans/
- relay-freshness.md  ◀
- status.md
- todo.md

Recent

- status.md · 2m
- relay-freshness.md · 14m

### relay-freshness.md

root / research / relay-freshness.md

**Relay Freshness Protocol**

The freshness protocol tracks per-relay event recency and surfaces staleness
before it becomes user-visible drift. Each relay entry carries a
last-confirmed timestamp...

Backlinks: plan.md, status.md

### Outline

- Overview
- Design goals
- Phase 1 — tracking  ◀
- Phase 2 — surfacing
- Verification

## Layout Notes

- Sidebar: file tree on top, Recently Updated folded in beneath it — the home-page card dissolves into the persistent rail.
- Current file is highlighted in the tree (◀ marks it in the sketch); ancestor folders auto-expand.
- Center column: breadcrumbs + copy-path stay at the top of the pane; source/rendered toggle stays top-right of the document.
- Outline: generated from the rendered document's headings; scrollspy highlights the section in view (◀); clicking scrolls smoothly.
```

---

## 3. Home View

With the tree living permanently in the sidebar, the home (center) pane is
freed to become a landing surface oriented around activity: Recently Updated
becomes the primary content, followed by an at-a-glance corpus summary.

```wiremd
## Home View {.grid-2}

### Files

- research/
- plans/
- progress/
- status.md
- todo.md

### thoughts/

**Recently Updated**

- status.md · 2 minutes ago · progress/
- relay-freshness.md · 14 minutes ago · research/
- implementation-plan.md · 1 hour ago · plans/
- todo.md · 3 hours ago

**Corpus**

142 markdown files · 12 directories · last change 2 minutes ago

[Follow latest changes]{.primary}

## Home Notes

- The "Follow latest changes" button is the discoverable on-ramp to follow mode.
- Recently Updated entries keep the existing keyboard navigation (arrow keys, enter).
- The corpus line replaces the bare root-path heading — same information, more context.
```

---

## 4. Follow Mode

The signature new capability. When enabled, the browser tails the corpus: any
file-change event navigates the reading pane to the changed file. When an
agent is writing documentation, the operator watches the work appear. The
toggle lives in the header; active state is unmistakable.

```wiremd
::: navbar
## stigmergic
[Search  Ctrl+K]{.secondary}  [Following]*  [Theme]{.secondary}  [?]{.secondary}
:::

::: card
### Following

Watching 142 files — last event: status.md, 4 seconds ago.

Navigated here automatically. Any manual navigation pauses following;
re-enable from the header or press F.

(*) Follow entire corpus  ( ) Follow current directory only

[Pause]{.secondary}
:::
```

Behavior notes:

- Reuses the existing SSE change events — no new server machinery; the client
  navigates to the changed path instead of merely re-rendering the current one.
- Manual navigation (click, palette, back button) pauses follow rather than
  fighting the user; a subtle "paused" pill in the header offers one-click resume.
- Scope radio (corpus vs. current directory) covers the case where multiple
  agents write to sibling trees.
- Rapid successive changes debounce; the newest event wins.

---

## 5. Command Palette — Full-Text Search

The palette grows a third result group: content matches with context snippets.
Filename fuzzy-match (Fuse) stays for the Files group; content search hits a
new server-side index over the markdown corpus, queried as you type.

```wiremd
::: modal
## Search

[freshness_____________________________]

**Content — 3 matches**

research/relay-freshness.md — "…the **freshness** protocol tracks per-relay event recency…"

plans/implementation-plan.md — "…**freshness** checks run on the hourly sweep…"

progress/status.md — "…deployed the **freshness** watcher to prod…"

**Files — 1 match**

relay-freshness.md

**Commands**

Toggle follow mode

Toggle theme

Copy current file path
:::
```

Design notes:

- Content group ranks above Files when the query looks like prose (length > 2 words
  or no filename-ish characters); otherwise Files ranks first.
- Snippets show ~80 chars of context with the match emphasized; enter navigates
  to the file (heading-anchor jump when the match falls under a known heading).
- Commands group gains the new surface actions: follow toggle, theme toggle.

---

## 6. Keyboard Shortcuts Overlay

Triggered by the header "?" button or pressing `?` outside an input. Cheap to
build, large payoff in discoverability for everything above.

```wiremd
::: modal
## Keyboard Shortcuts

Ctrl+K — open search palette

F — toggle follow mode

T — cycle theme

S — toggle source view

Arrow keys — navigate file lists

? — this overlay

Esc — close overlays
:::
```

---

## Typography and polish (applies across all mockups)

- Reading measure capped near 72ch; current full-width prose is the single
  largest legibility defect.
- Slightly larger base font for prose (16→17px), tightened heading scale.
- Chrome (sidebar, rails, header) drops to a smaller type size than content,
  visually separating navigation from reading.
- Frontmatter metadata card collapses to a single summary row, expandable —
  it currently outcompetes the document it annotates.
