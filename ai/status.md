# Status: stigmergic.dev

Daily work log. Add entries under date headers (## YYYY-MM-DD) after each unit of work.

## 2026-07-30

### Wiki-link embed transclusion, phase 2, `feature/wikilink-embed-transclusion`

`![[note]]` and `![[note#section]]` now render their target inline.

- `internal/markdown/embed.go` holds the extension. `embedTransformer` runs at priority 100 and replaces a block whose sole child is an embed with an `embedBlock`, accepting both `ast.Paragraph` and `ast.TextBlock` as the container, since a tight list item wraps its content in a `TextBlock` and an embed alone in a list item is an ordinary vault form. An embed with text beside it is left in the inline stream and still renders as an anchor, which `WikilinkRenderer.enter` now carries a comment about so it does not read as an oversight.
- The renderer resolves the target through the same `wikilink.Resolver` ordinary links use, passing a node with no fragment so the resolver returns a bare route and no anchor has to be trimmed back off. It slices the section, then recurses through `Parse`, which allocates its own buffer and its own `parser.Context`. That is not incidental: `renderer.Render` calls `Flush` on the writer it is handed, and reusing the host's context would let the embedded note's frontmatter overwrite the host's.
- The visited key is route plus fragment and is popped on the way out, so the guard tracks the path to the current node rather than the whole page. Two sibling embeds of the same section both render; a genuine cycle terminates at a marker. `MaxEmbedDepth` is a named constant and acts as a backstop for depth alone.
- Every failure path writes a visible marker carrying `data-embed-error` set to `unresolved`, `no-section`, `cycle`, or `depth`, never an error. Roughly half the link targets in a real vault are dangling, so an unresolved embed is an ordinary outcome and not a reason to fail a page.
- `Parse` gained a third parameter, a nil-disableable `*EmbedContext`, mirroring the nil-resolver idiom above it. A nil context reproduces the previous output exactly, which a test pins. Nested renders also omit `AutoHeadingID`, so transcluded headings carry no `id` and cannot collide with host ids or capture the outline rail's scrollspy anchors.
- Styling went into `sharedThemeRules` in `layout.templ`, not into `input.css`. Two style trees exist and only one is live: `internal/embed/static.go` embeds `internal/embed/web/static`, which is what `make css` compiles and what `/static/styles/output.css` serves. The tree at the repository root, `web/static/`, is tracked but dead, and its `.wikilink-unresolved` rule has never reached a compiled bundle. That rule was added to `sharedThemeRules` alongside the transclusion rules, since the unresolved marker depends on it.

Verified: lint, test, and build green, including a handler-level integration test that drives transclusion through the wired embed source and resolver.

## 2026-07-29

### Wiki-link embed transclusion, `feature/wikilink-embed-transclusion`

`![[note#section]]` is parsed today and then discarded: the vendored parser sets `Node.Embed` and splits the fragment, and `WikilinkRenderer.enter` never reads either. Five phases give the leading `!` its meaning. Plan at `thoughts/plans/2026-07-29_10-49-49_wikilink-embed-transclusion.md`, research at `thoughts/research/2026-07-29_09-18-46_wikilink-embed-transclusion.md`.

**Phase 1, section location and content lookup.** Two pure files, no rendering change.

- `ExtractSection` in `internal/markdown/section.go` returns the byte range of the section a fragment names. The range starts at the heading's own line, recovered by scanning back to the preceding newline so an ATX marker or a setext underline survives into the slice, and ends where the next heading of the same or a higher level begins, so descendant subsections are included by construction. Its parse-only goldmark instance mirrors `ExtractOutline` plus one addition that is load-bearing: the wikilink inline parser at priority 199. Without it, `## [[03-02-23]]` flattens to its literal brackets, and ten of the forty-seven section embeds in the operator's vault never match. Matching is exact and case-sensitive across the whole document first, case-insensitive only as a fallback, with no prefix or substring pass, because `principles of psychology` carries both `#### floodlights` and `#### on the floodlights` and the vault embeds the former.
- `ClassifyEmbedTarget` sorts a target into note, image, or attachment by extension, with the image set copied from the wikilink package's own `resolveAsImage` list. A trailing dotted run containing a space is not treated as an extension: `filepath.Ext` reports `.2 release` for the note title `v1.2 release`, and classifying that as an attachment would send a resolvable note to the filesystem probe and render a marker instead of its content.
- `FSEmbedSource` in `internal/markdown/embedsource.go` is the target-to-content seam a `wikilink.Resolver` does not provide. Notes are read with `fs.ReadFile` through the same filesystem the handler already holds rather than through `Server.index.corpus`, which sits behind `indexMux` and can lag the filesystem during a rebuild. Assets are found by probing the content root and then a configured attachment root, deliberately without an index, which is what keeps `source.Scan`, the tree, the sidebar, and the search corpus free of binary entries. Both methods reject paths failing `fs.ValidPath`, so a target cannot escape the content root.

Verified: lint, test, and build green, 41 new subtests passing under `-race`.

## 2026-07-25

### UI latency remediation, four phases on `feature/sidebar-structural-refresh`

The local instance serves the whole buildtall home (6,030 markdown files, 76 MB) and had become unusable interactively. Two independent amplification loops compounded: the browser rebuilt a 5.7 MB, 51,500-node sidebar on every filesystem event, and the server re-indexed the entire corpus once per event with no coalescing. Baseline `GET /` was 6,668,404 bytes; the service had burned 6 h 19 m of CPU and 16.5 GB of egress.

Four phases, ordered by relief per unit of risk, one commit each.

**Phase 1, conditional sidebar refresh, committed `7e0d6b5`.** `Tree.Signature()` renders every node's path and type in traversal order, excluding mod times, so it changes when the corpus gains, loses, or renames an entry and never when a file's contents change. It is the shape material itself rather than a hash, so comparison is exact equality and no collision can silently suppress a refresh. `updateTree()` returns a structural verdict, `sseEvent` gains `Structural bool` with no `omitempty`, and `events.js` refetches the tree only when `msg.structural !== false`, otherwise swapping the new `/partial/recent` fragment into a standing `#recent` container. The `index-ready` branch refreshes the sidebar explicitly: it never did before, so a page loaded during the background scan sat on "Indexing..." until a blind reload, and Phase 1's gate would have made that permanent.

**Phase 2, event coalescing, committed `155317d`.** `broadcastEvents` accumulates instead of rebuilding per event, with a 300 ms quiet window and a 2 s ceiling so sustained writes still flush. Tests drive a `fakeWatchableSource` over `fstest.MapFS` directly into the broadcast loop, measuring coalescing without a real watcher and its own per-path debounce in the path.

**Phase 3, incremental index rebuilds, committed `94846bb`.** Rebuild cost now tracks what changed. Two findings shaped it. The plan's mod-time comparison was unusable: `SearchableFile.ModTime` is Unix seconds, so a one-character edit within the same second looks unchanged and would leave indexes silently stale; the working stamp is nanoseconds plus size. And the right split is parsing versus resolution, not changed versus unchanged: a wikilink names a page rather than a path, so adding or removing any file changes what every other file's links resolve to. Caching unresolved link refs makes the expensive half cacheable and removes the staleness class outright. Also fixed: `sort.Slice` is not stable, and backlinks sorted by title alone left same-named sources in different directories unordered, which made incremental and full builds differ for no reason.

**Phase 4, lazy tree rendering, committed `85bd2d7`.** The tree shipped fully expanded because `pruneEmptyDirectories` guarantees every surviving directory has markdown descendants and the template keyed expansion on exactly that. Directories now ship collapsed except those containing the current file, with the rest arriving from `/partial/tree/` one directory at a time on first expand.

- `models.ExpandedDirs` names the directories a render materializes; `models.AncestorDirs` derives that set from the current file, so a full page load shows the current row without a round trip. `models.TreeView` bundles tree and expansion, which kept `Layout`, `Home`, `Directory`, and `Markdown` at their existing arity.
- `/partial/sidebar` accepts `?path=`, so a structural refresh returns the tree already opened to the displayed file. The path only seeds the expansion and touches no filesystem, so a non-canonical one is dropped and the tree renders collapsed rather than the request failing.
- `nav.js` loads a placeholder once on first expand via `htmx.ajax`, which returns a real Promise in this 2.0.4 build. `fetch` would leave `hx-get` unprocessed and turn tree clicks into full page loads. `revealPath` walks the chain in sequence, because a nested placeholder does not exist until its parent's rows arrive; `syncCurrentFile` falls back to it when the row is absent, which is the `#content`-only navigation case.
- Tree glyphs moved to a `<use>` sprite emitted once per document. The symbol ids are `tree-icon-*`: the command palette reads its own icon markup via `getElementById('icon-file')`, and the sprite is emitted earlier in the body, so a shared id would have handed the palette a `<symbol>`. A test pins the two sets disjoint.
- Removing the tree left the inline `markdown-files` JSON as the bulk of the page at 882 KB. The palette already refetched the same list from `/api/files` after any swap, so it now fetches at init and the script is gone, taking `Layout`'s `files` parameter with it. The palette's `afterSwap` refetch is scoped to `#content`, `#sidebar`, and `#recent`, its prior trigger set, so directory expansion does not drag the list behind it.
- A directory page listing renders one level now, expandable, where it used to dump its whole subtree into the reading pane. Same component, same reasoning.

Result: `GET /` against the buildtall home falls from 6,668,404 bytes to 55,252, a 121× reduction. The sidebar span alone goes from 5,742,332 to 10,660. A page opened to a deep file costs 64,451.

Two notes on how the work went. Extracting the canonical-path check out of `handleMarkdown` into a shared helper broke gosec's taint analysis, which follows an inline sanitizer but not one behind a call; the check was restored inline with a comment saying why, rather than suppressed. And a `..` in a tree path never reaches the handler at all, since `ServeMux` cleans and redirects first, so the test asserts what actually happens rather than an unreachable 404.

Verification: lint, test, and build green via run_silent under `nix develop -c` at every phase; suites run three times under `-race` at Phases 2 and 3 with no flakes. All three modified JS files pass `node --check`. **Operator runtime verification in a browser is outstanding for all four phases**, covering: an edit issuing no `/partial/sidebar` request; a new file still updating the tree; a bulk checkout settling once; whether the 300 ms window reads as sluggish; the tree showing collapsed and expanding without a stall; a deep file reached by URL showing its ancestors; arrow-key navigation; the palette still finding files in unexpanded subtrees; sprite glyphs rendering; and the palette opened immediately on a cold load, where files now arrive asynchronously.

Next: operator runtime verification, then push and PR to develop.

## 2026-07-03

### Follow mode persistence: auto-pause now opt-in, committed `410bed7`

Follow auto-paused on every user-initiated `#content` load (`htmx:beforeRequest`) and on `popstate`, which the operator experienced as the toggle flipping off whenever a new document was opened. Decision: follow stays on until manually toggled; auto-pause becomes an opt-in behavior.

- `follow.js`: new persisted `autoPause` preference (localStorage `stigmergic-follow-autopause`, default off) gates both pause triggers; `setAutoPause(value)` persists and, when disabling while paused, clears `paused` so follow resumes immediately (no stranded pause the UI can't explain). The `selfNav`/`stigmergicProgrammaticNav` exemptions remain for when auto-pause is on.
- `layout.templ`: follow dropdown gains a "Behavior" section below the scope radios — "Auto-pause on navigation" checkbox with a tooltip explaining the pause-and-F-to-resume behavior.

Verification: generate/lint/test/build green via run_silent; operator verified in LibreWolf (persistence across navigation and back/forward, opt-in pause + F resume, resume-on-disable, reload persistence, tooltip).

### Fix: keyboard scrolling of the document pane, committed `4274975`

Post-redesign regression: the body became a fixed viewport (`h-screen overflow-hidden`) with independent scroll panes, so native key scrolling (arrows, PageUp/Down, Home/End, Space) no longer reached the document — `#content` was never focusable, and focus sat on the clicked sidebar anchor after navigation. Fix restores native browser scrolling rather than reimplementing keys in JS:

- `layout.templ`: `tabindex="0"` on `<main id="content">` (scrollable regions must be keyboard-focusable per WCAG/axe); `#content:focus { outline: none }`.
- `events.js`: new `focusContent()` — focuses the pane with `preventScroll: true`, skipping `/` where `initKeyboardNav` owns focus for sidebar tree nav — called at initial load, after each `#content` htmx swap, on bfcache `pageshow`, and on `htmx:historyRestore`.

Verification: generate/lint/test/build green via run_silent; operator verified in LibreWolf (document load, sidebar navigation, back/forward). Known caveat: clicking non-interactive text inside the pane focuses it in Chromium but may drop focus to `<body>` in Firefox forks; a `mousedown` refocus handler is the one-line follow-up if it bites.

### Possible improvement: palette search ranking (filename precedence + quality/recency)

Investigation of the Phase 4 search surfaced two ranking gaps, noted here as a candidate future unit (not scheduled):

- **Group order is a query-shape heuristic, not match-driven** (`command-palette.js` `applyResults`): >2 words or no `[./_-]` chars puts Content above Files regardless of how strong the filename hit is. Improvement: order Files first whenever Fuse returns a strong filename match (scores already captured via `includeScore`), Content first only when filename hits are absent/weak; dedupe Content rows whose path already appears in Files.
- **No composite quality × recency ranking**: Files rank by Fuse score only (recency unused); Content ranks by mod-time only (`search.go` scans the mod-time-descending index, first occurrence per doc, no occurrence count / title boost / whole-word bonus). Improvement: expose `SearchableFile.ModTime` through `/api/files` + `markdown-files` JSON and sort files by score+recency composite; extend the server scan to score candidate docs (occurrences, title hit, whole-word) with recency decay before the cap of 20.

Contained scope: `command-palette.js` + `internal/server/search.go`; no new libraries.

### UI Redesign — Phase 5 Complete (theme toggle, typography, login theming), committed `a6735ec`

Phase 4 committed as `eb23587` after operator verification. Phase 5 on `feature/ui-redesign` — all five plan phases now committed and operator-verified:

- **Theme system** (`internal/theme`): `Theme` gains `ChromaStyle`/`MermaidTheme` pairings (TOML keys: iceberg-dark → nord/dark, iceberg-light → github/default) and a load-time `ChromaCSS` — the chroma stylesheet generated per theme and scoped under `[data-theme="name"]` (new `chroma.go`; WriteCSS output shape verified empirically before implementation; unknown chroma styles fail at load). New `LoadEmbedded()` loads every embedded palette.
- **Layout** (`layout.templ`): `ThemeCSS(thm, themes)` emits `:root` boot vars + per-theme `[data-theme]` variable blocks + each theme's scoped chroma CSS + shared component rules; `PrePaintScript` (the single deliberate inline script, first in `<head>`) applies the localStorage theme attribute before first paint; `theme-config` JSON registry (boot/order/mermaid map); mermaid init switched to `startOnLoad: false` with theme-aware selection; header gains a Theme control between Follow and `?`. `Layout` signature threads `themes []*theme.Theme`; Home/Directory/Markdown templates and server call sites updated (`Server.themes` from `LoadEmbedded`, boot theme prepended when custom).
- **Class-based highlighting** (`parser.go`): `html.WithClasses(true)` replaces inline nord styles; the four inline-style test assertions became class-based contracts plus an explicit no-inline-styles check.
- **Client JS**: `ui.js` — `applyTheme`/`cycleTheme`/`handleThemeKeydown` (`T`, input-guarded); `render.js` — mermaid source stashed in `data-mermaid-src` on first render so theme toggles re-render from source; `events.js` — key handler + initial mermaid render wired.
- **Typography** (`markdown.templ`, `input.css`): article measure `max-w-none` → `max-w-[72ch] mx-auto`; frontmatter card collapses to a "Frontmatter" summary row (Alpine chevron expand); input.css grays reconciled to theme variables (border compat block included), dead `btn`/`btn-secondary`/`markdown-content` utilities removed, chrome-vs-content scale added (13px sidebar/outline, 17px prose).
- **Login theming**: `LoginLayout(thm, themes)` emits shared `ThemeCSS` + pre-paint script; all hardcoded hex replaced with variables; `auth.LoginHandler` threads themes.
- **Tests**: theme loader (embedded set, scoped chroma CSS, unknown theme/style failures), layout theme-switching scaffolding (scoped blocks, pre-paint, theme-config, no `startOnLoad: true`), login theme-variable assertions.
- **Help**: `T — Cycle theme` row.

Verification: generate/css/lint/test/build all green via run_silent (govet shadow findings in theme.go fixed properly, tests race-clean). Operator manually verified both themes, flashless load, chroma+mermaid re-theme, and login page. Committed as `a6735ec`.

Context note: `goldmark-highlighting` cloned to `ai/context/goldmark-highlighting/` (monorepo root) and registered in the root context index.

Next: buildtall-sop → push approval → PR to develop.

## 2026-07-02

### UI Redesign — Phase 4 Complete (full-text search), awaiting operator verification

Phase 3 committed as `0c752a6` after operator verification. Phase 4 on `feature/ui-redesign`:

- **Single-pass corpus read**: new `markdown.ReadCorpus` reads every file once per rescan; `BuildBacklinkIndex` now takes the pre-read map instead of an `fs.FS` (5 test call sites updated); new `rebuildIndexes` helper on Server feeds file list, backlink index, and the new search index from that one read (`initialScan` + `updateTree` both use it).
- **Search index** (`internal/server/search.go`, new): `searchIndex` holds original + lowercased content per doc in mod-time order; `search()` returns first case-insensitive occurrence per doc, capped at 20 with a truncation flag; snippets window ±40 bytes widened to rune boundaries, line breaks flattened byte-preserving so match offsets stay valid; non-ASCII-width case pairs fall back to the lowered copy for offset correctness.
- **`GET /api/search`**: registered unconditionally (serve + site modes); JSON `{query, results:[{path,title,snippet,matchStart,matchEnd}], truncated}`.
- **Palette Content group**: sequenced debounced fetch merged into a new `displayGroups` rendering (template's two static sections replaced by one dynamic group loop); ranking per mockup 5 — Content above Files when the query is prose-like (>2 words or no `[./_-]` characters); snippets render with `<strong>` match emphasis via offsets (HTML-escaped around); selection navigates via the established htmx path; stale responses discarded by sequence + query check; empty-state and placeholder copy updated.
- **Tests**: unit — match+snippet offsets, case-insensitivity, boundary windowing (whole-doc snippet), 25-doc cap+truncation, empty/whitespace/no-match queries; integration — `/api/search` over both `FilesystemSource` and `EmbeddedSource`, and rebuild-on-change (new file's content searchable after watcher rescan).

Verification: generate/lint/test/build all green via run_silent (lint 0 findings after goconst/prealloc fixes, tests race-clean). Halted for operator manual verification before commit.

### UI Redesign — Phase 3 Complete (document outline + scrollspy), awaiting operator verification

Phase 2 committed as `a41e4c7` after operator verification. Phase 3 on `feature/ui-redesign`:

- **`internal/markdown/outline.go`** (new): `ExtractOutline` — AST walk collecting level/text/anchor-id per heading, per the backlinks.go pattern. Parser mirrors `Parse`'s heading-affecting config (AutoHeadingID + frontmatter extender, so metadata blocks aren't misread as setext headings and ids match the rendered HTML exactly). Heading text flattened via manual inline-node recursion (no deprecated `Node.Text`). `OutlineEntry` model in `internal/models` alongside `BacklinkEntry`.
- **Outline rail rendering**: new `components.Outline` (nav list, level-indented, `data-outline-target` per entry) rendered two ways — full pages via a new `outline` param on `Layout` (Home/Directory pass nil), htmx partials via `components.OutlineOOB` (`hx-swap-oob="innerHTML"` against the persistent `#outline` aside), appended by a shared `renderOutlineOOB` helper after every `#content` partial: entries for markdown, nil for home/directory (clears the rail).
- **Scrollspy** (`nav.js`): IntersectionObserver rooted on `#content` (the scroll container, not the viewport), `-60%` bottom rootMargin; active = first visible heading, falling back to the last heading scrolled past; outline clicks smooth-scroll via `scrollIntoView`. Re-inited on DOMContentLoaded, `#content` afterSwap, and `#outline` oobAfterSwap (htmx OOB ordering relative to the main swap is not assumed). `.outline-link`/`-active` colors in themeCSS.
- **Tests**: `ExtractOutline` table tests (nested levels, duplicate slugs → `-1` suffix, setext, empty, inline formatting — goldmark's id slug includes link destinations, verified against real behavior — frontmatter-not-a-heading); template tests for rail markup + anchor hrefs and empty-rail on non-document pages; handler tests asserting the OOB fragment with known heading in markdown partials and cleared rail in home partials.

Verification: generate/css/lint/test/build all green via run_silent (lint 0 findings, tests race-clean). Halted for operator manual verification before commit.

### UI Redesign — Phase 2 Complete (structured SSE + follow mode), awaiting operator verification

Phase 1 committed as `a3a13a0` after operator verification. Phase 2 on `feature/ui-redesign`:

- **SSE JSON envelope**: `broadcastEvents`/`broadcastIndexReady`/`broadcastReload` now push `{"type":"reload","path":"..."}` / `{"type":"index-ready"}` (path omitted when empty = refresh regardless) via a shared `encodeSSEEvent` + `broadcast` helper pair; the three hand-rolled client loops collapsed into one.
- **Corpus-relative event paths** (defect found while testing): `FilesystemSource.pump` was forwarding the watcher's absolute path; clients need corpus-relative to build `/file/` URLs. Now relativized against the source root with `filepath.Rel` + `ToSlash`; `source.Event` doc updated to state the contract, source test pins it exactly.
- **`FollowMode` capability**: new `UICapabilities` flag set from the `Watchable` assertion — embedded/site mode renders no toggle.
- **Header Follow control** (caps-gated): toggle button (Off / Following in green / Paused in orange, one-click resume) with `data-follow-toggle` carrying `hx-push-url="true"` as the htmx.ajax inheritance source, plus a scope dropdown (entire corpus vs current directory).
- **`js/app/follow.js`** (new): Alpine store — localStorage persistence (`stigmergic-follow`, `stigmergic-follow-scope`), `F` keybinding, 150ms newest-event-wins debounce atop the server's watcher debounce, subtree-prefix directory scoping, pause on any user-initiated `#content` request or popstate (SSE refreshes flagged via `stigmergicProgrammaticNav`), same-file navigation skips the history push.
- **events.js dispatcher**: parses the envelope with bare-string legacy fallback; sidebar always refreshes; follow store gets first claim on the event; otherwise `#content` refetches only when it shows the changed path (the file itself, an ancestor directory listing, or home).
- **help.templ**: `F` row added.
- **Tests**: exact payload-shape assertions for change/reload/index-ready broadcasts, FollowMode true for filesystem + false for embedded, SSE stream framing integration test (end-to-end: file write → framed JSON envelope on the wire), template test gating the toggle on the capability.

Verification: generate/lint/test/build all green via run_silent (lint 0 findings, tests race-clean). Halted for operator manual verification before commit.

### UI Redesign — Phase 1 Complete (layout substrate), awaiting operator verification

On `feature/ui-redesign` (from develop), per `thoughts/plans/2026-07-02_15-17-17_stigmergic-ui-redesign.md`:

- **Three-pane shell**: `layout.templ` body is now header + flex row of persistent `#sidebar` (new `components.Sidebar` — tree + compact Recent list, collapsible via header chevron, hidden below `md`), `#content` (the sole htmx swap target replacing `#main`, independent scroll container), and `#outline` (empty placeholder rail, hidden below 1100px). Layout signature gains `tree` + `recentFiles`.
- **Header refined**: live indicator removed (`live_indicator.templ` + generated file deleted; `.indicator-*` CSS and keyframes dropped from themeCSS); right cluster gains Search (dispatches `toggle-palette`, palette listens) and `?` (new `components.Help` overlay with Ctrl+K/S/arrows/?/Esc rows, `helpOverlay()` Alpine factory).
- **Inline JS extracted** to `internal/embed/web/static/js/app/{render,nav,events,ui}.js` — behavior-preserving split of the ~220-line layout.templ script block. New in nav.js: `syncCurrentFile()` highlights the current file in the tree (`data-path` attributes, `.tree-item-current` style) and auto-expands ancestor folders. events.js keeps the bare-string SSE contract but refetches `#content` and refreshes `/partial/sidebar`; `S` now toggles source view via a window event. scrollProgress tracks `#content` scroll (body no longer scrolls). tailwind.config.js gains the JS dir in content globs so extracted class strings survive `make css`.
- **Navigation normalized**: breadcrumbs (markdown + directory), tree, recent, and palette file-selection all swap `#content` with push-url; palette uses `htmx.ajax` sourced from its root carrying `hx-push-url="true"` (htmx resolves inherited attributes from the ajax `source`). Inline `onmouseover/onmouseout` handlers dropped from recent.templ (covered by existing `[data-nav-item]` CSS).
- **Home is an activity surface**: tree moved out of `HomeContent`; Recently Updated primary, corpus summary line (file/dir counts + last change) via new `countDirs` and shared `uiData()` helper in handlers.go; `/partial/sidebar` route added for scoped SSE refresh.
- **Tests**: layout landmark/indicator-absence assertions, tree `data-path`/`#content` targeting, breadcrumb htmx quartet, sidebar-partial handler test, htmx content-partial purity test (no sidebar markup, no full page).

Verification: generate/css/lint/test/build all green via run_silent (lint 0 findings, tests race-clean). Halted for operator manual verification before commit.

### ContentSource Abstraction — Phase 4 Complete (v0.3.0 released, prod cut over to embedded site)

- **Phase 3 committed** as `f08689d` after operator review of site pages and the `serve ./example` demo check.
- **Merged and released**: `feature/content-source` fast-forwarded into `develop` and `master` (no PR, operator-directed), both pushed. Version bumped to 0.3.0 (`e3e8a40`). Tagged `v0.3.0`; goreleaser published binaries for linux/darwin amd64+arm64 and windows/amd64. Release required two fixes: goreleaser must run inside the devshell (Makefile release target now uses `nix develop -c`, committed `e138fb8` on develop), and the active `gh` account (spindle-bot) has an invalid keyring token — published with the plantimals token instead. spindle-bot re-auth still pending.
- **Deploy-repo cutover** (buildtall monorepo, `1d83917` on develop): stigmergic flake input pinned to `refs/tags/v0.3.0`; `ExecStart` → `stigmergic site --config /etc/stigmergic/config.toml` (source-tree `/example` reference gone); config.toml drops `respectgitignore`. Nonprod rehearsal exposed a develop-wide gap — listoflists-web, sayer-gateway, and sayer-worker had no nonprod secrets and crash-looped, tripping deploy-rs rollback; disabled all three in `nonprod-overrides.nix` per the catallaxy precedent.
- **Verified on nonprod, then prod** (`https://stigmergic.dev`): root 302 → `/file/index.md`, embedded index renders, `/file/img/stigmergic.png` 200 (114931 bytes), `/file/demo.md` 200, `.stigmergic.toml` and `.gitignore` 404. The example/-as-public-site leak hazard is closed.

## 2026-07-01

### ContentSource Abstraction — Phase 3 Complete (content curation + docs)

Phase 2 committed as `f1911c9` after operator site-mode verification. Phase 3 on `feature/content-source`:

- **`site/content/` curation**: index de-duplicated (single wikilink nav section, notes the site is self-hosted by `stigmergic site`), config example corrected to real Viper key (`defaultfile`); installation gains a pre-built-binaries section and a CLI reference covering root/serve/site with the source-model distinction; features gains Wikilinks & Backlinks and UI Wireframes sections (shipped features the page omitted); architecture documents the two-command/one-pipeline model, the ContentSource capability design, and a corrected project structure (`internal/source`, `site/`, example as demo corpus). demo.md's fabricated Nostr entities replaced with the real project npub (fake note reference dropped).
- **`example/` demoted to demo corpus**: the four website pages removed; new README-style `index.md` describes the corpus, demonstrates inline image rendering, and points at `stigmergic site` for the public website. Keeps `demo.md`, `img/`, `.stigmergic.toml`.
- **README**: documents bare-command default, the two content-source commands, and the `ContentSource` capability model; config example corrected to actual Viper keys (`loglevel`, `respectgitignore`, `ignorepatterns`, `defaultfile`, `base_url`); env var example corrected (`STIGMERGIC_LOGLEVEL`).
- **`ai/next-phase.md` deleted** — superseded by the ContentSource plan; it misdescribed the design.

Lint 0 issues, tests race-clean, build green. Awaiting operator review of site pages (voice + accuracy) and `serve ./example` demo check, then commit.

### ContentSource Abstraction — Phase 2 Complete (embedded site mode)

Phase 1 committed as `22c8e43` after operator serve-parity verification. Implemented Phase 2 on `feature/content-source`:

- **`site/` package**: `//go:embed all:content` over `site/content/` (index, installation, features, architecture, demo pages plus `img/`, copied verbatim from `example/` — curation is Phase 3); `Content()` accessor returns `fs.Sub` of the content root. No dotfiles in the embedded tree.
- **`internal/source/embedded.go`**: `EmbeddedSource` implements only the core `ContentSource` interface — asserts none of Watchable/GitignoreAware/Timestamped/Rooted; no-op Close. Tests assert capability absence and exercise tree building over `fstest.MapFS` and the real embedded site FS.
- **`cmd/stigmergic/site.go`**: `stigmergic site` subcommand (no positional args); port negotiation identical to serve; honors loaded config wholesale including auth; source named "stigmergic.dev" for display. Registered on root.
- **Server integration tests** (`internal/server/embedded_test.go`): full request path over an embedded source — markdown render, PNG serve via `ServeFileFS`, directory listing, `/api/files`; gitignore endpoints 404; traversal attempts (dotdot, encoded, absolute, trailing slash) rejected; index-ready fires and shutdown is deadlock-free without watcher goroutines. No server code changes were needed — Phase 1 architecture already handled non-watchable sources.

Lint 0 issues, tests race-clean, build green. Awaiting operator manual verification of `./stigmergic site`, then commit.

### ContentSource Abstraction — Phase 1 Complete (pure refactor)

Implemented Phase 1 of the ContentSource plan (`thoughts/plans/2026-07-01_22-25-38_stigmergic-contentsource-embedded-site.md`) on `feature/content-source`:

- **`internal/source`**: new package. `ContentSource` interface (FS/Name/Close) with capability interfaces asserted by the server: `Watchable` (pre-classified change events), `GitignoreAware` (runtime toggle), `Timestamped` (meaningful mod times), `Rooted` (local absolute root — gates the copy-path button). `FilesystemSource` implements all four, absorbing watcher construction and event classification from `NewServer`/`broadcastEvents`.
- **Scanner**: rewritten fs-generic over `fs.WalkDir` in `internal/source`; reads `.gitignore` through the source FS; route-relative forward-slash node paths; relocated from `internal/watcher`.
- **`Tree.Find`**: now route-relative; `Tree.RootPath` and the `filepath.Abs`/`Rel` round-trip removed.
- **Server core**: `NewServer(cfg, src)`; broadcast goroutine only for `Watchable` sources (WaitGroup arithmetic adjusted); gitignore endpoints registered only for `GitignoreAware`; shutdown closes the source. `BuildBacklinkIndex` takes `fs.FS`.
- **UI capability flags**: `models.UICapabilities` threaded through Layout/Home/Directory/Markdown templates plus `data-gitignore-enabled` body attribute consumed by the command palette JS. All flags true for `FilesystemSource`, so serve-mode rendering is unchanged.
- **Handler hardening**: explicit `path.Clean` + `fs.ValidPath` canonical-path gate in `handleMarkdown` (behavior-preserving; satisfies gosec taint analysis).
- **Lint driven to zero** (36 findings fixed across source, server, timeutil, markdown, models, and their tests). Tests race-clean, build green.

Awaiting operator serve-mode parity verification, then commit.

## 2026-03-26

### WireMD Integration (#21) — Implementation Complete

Implemented wiremd fenced code block rendering across 3 phases on `feature/wiremd-integration`:

- **Phase 1**: Built wiremd browser bundle (IIFE via esbuild) with Node.js shims for browser compatibility. Added `wiremd-js` Makefile target, integrated into `vendor-js` workflow.
- **Phase 2**: Created goldmark AST extension (`internal/markdown/wiremd.go`) — transformer intercepts wiremd fenced code blocks, renderer emits `<pre class="wiremd">`. Registered before syntax highlighter. Unit tests cover rendering, HTML escaping, and non-interference with other block types.
- **Phase 3**: Wired client-side rendering in `layout.templ` — `renderWiremd()` function handles DOMContentLoaded and htmx:afterSwap. CSS injected once via deduped `<style id="wiremd-styles">`. Copy buttons skip wiremd blocks.

All acceptance criteria pass. Lint, test, build green. Awaiting operator browser verification and PR.

## 2025-11-16

### Standup (automated)

**First Assessment**

**Current State**:
- Git: 3 uncommitted files (CLAUDE.md, go.mod, go.sum), 2 untracked (README.md, ai/status.md)
- Tests: 10 test files present, status unknown
- Build: Binary exists (16MB, recent build)

**Project Overview**:
Go web application using goldmark for markdown parsing and rendering. Stack includes templ generation, Tailwind CSS compilation. Build toolchain: Makefile with css, generate, build, test, clean targets. Vendor directory present with dependencies.

**Signals**:
- Momentum: LOW - Last commit 21 days ago (2025-10-26), recent dependency updates in go.mod/go.sum uncommitted
- Blockers: None detected, build artifacts present
- Ready for work: YES - Development environment configured, dependencies vendored

Suggested action: Run test suite to verify current state, then review and commit outstanding changes (dependency updates, documentation).

## 2025-12-17

Implemented three markdown view enhancements:

1. **Scroll progress indicator** - Fixed position bar at top of page showing scroll position. Uses Alpine.js with passive scroll listener for performance. Recalculates on htmx:afterSwap.

2. **Copy button on code blocks** - DOM post-processing adds copy buttons to all `<pre>` elements (except mermaid diagrams). Shows "Copied!" feedback for 2 seconds. Re-initializes on htmx:afterSwap.

3. **Raw markdown toggle** - "Source/Rendered" button to toggle between rendered HTML and raw markdown text. Handler now passes both `content` (rendered) and `rawContent` (source) to templates. Resets to rendered view on htmx:afterSwap.

Files modified:
- `web/templates/components/layout.templ` - scrollProgress(), initCodeCopyButtons(), rawToggle() functions; scroll bar markup
- `web/templates/markdown.templ` - Updated signatures to accept rawContent; added toggle UI
- `internal/server/handlers.go` - Pass string(content) as rawContent to template calls

Verification: `make generate && make build && make test && make lint` - all pass (0 lint issues, all tests pass).

## 2025-12-18

Upgraded Tailwind CSS from v3.4.1 to v4.1.18 using official upgrade tool.

Migration changes:
- CSS syntax: `@tailwind base/components/utilities` → `@import 'tailwindcss'`
- Custom components: `@layer components` → `@utility` directives
- Class renames: `outline-none` → `outline-hidden`, `shadow-sm` → `shadow-xs`, `rounded` → `rounded-sm`
- Modifier order: `hover:prose-a:underline` → `prose-a:hover:underline`
- Added `@config` directive pointing to tailwind.config.js
- Installed `@tailwindcss/cli` package (v4 CLI is separate from core)
- Updated Makefile to use `pnpm exec tailwindcss` instead of direct `tailwindcss`
- Added border-color compatibility layer for v3→v4 default change

Build performance: CSS now compiles in ~239ms (v4 speed improvement).

Files changed: Makefile, package.json, pnpm-lock.yaml, input.css (both copies), 5 template files.

Verification: `make build && make test && make lint` - all pass.

---

Upgraded command palette visual design to match Alpine.js documentation style.

Five-phase implementation:
1. Added IconSearch, IconClose, IconReturn to icons.templ
2. Updated search header with magnifying glass icon (left) and close button (right)
3. Restructured results with category headers ("Commands", "Files"), item icons (terminal for commands, document for files), and return arrow on selected item using hidden template pattern
4. Added keyboard hints footer (↵ to select, ↓↑ to navigate, esc to close)
5. Implemented Fuse.js match highlighting with bold matched characters

Technical approach: Hidden `<template>` elements render icons once via templ; Alpine.js reads innerHTML at runtime. Single source of truth for icons, no duplication.

Files modified:
- `web/templates/components/icons.templ` - Added 3 new icon components
- `web/templates/components/command_palette.templ` - Complete UI restructure with categories, icons, footer, highlighting

Verification: `make build && make test && make lint` - all pass (0 lint issues).

## 2025-12-19

Implemented performance optimization eliminating multi-second delays on page load and command palette input.

**Phase 1: Server-side file list caching**
- Added `atomic.Value cachedFiles` field to Server struct for lock-free reads
- Initialize cache with `FlattenMarkdownFiles()` result on server startup
- Refresh cache atomically in `updateTree()` when watcher detects file changes
- Updated 3 handlers (`handleHome`, `handleMarkdown`, `handleFilesAPI`) to use cached load instead of calling `FlattenMarkdownFiles()` per-request

**Phase 2: Client-side query optimization**
- Updated command palette `filter()` function for single-character queries
- 1-char queries: fast prefix-only matching (no Fuse.js fuzzy search)
- 2+ char queries: standard Fuse.js fuzzy search
- Eliminates exponential fuzzy matching cost on single characters

**Performance impact** (tested with 5000 files):
- Page load: O(n log n) per request → O(1) cache lookup
- Command palette 1-char: exponential fuzzy → linear prefix filter
- Cache updates: atomic store on file system changes via SSE

Files modified:
- `internal/server/server.go` - Added atomic.Value field, init, updateTree refresh
- `internal/server/handlers.go` - 3 handlers now use cached files
- `web/templates/components/command_palette.templ` - Single-char prefix matching

Verification: `make test && make lint && make build` - all pass (0 lint issues).

---

Implemented background filesystem indexing to eliminate server startup blocking.

**Phase 1: Background scan infrastructure**
- Added `indexReady atomic.Bool` field to Server struct
- `NewServer()` now spawns `initialScan()` goroutine instead of blocking on scan
- Added `broadcastIndexReady()` to send SSE "index-ready" event when scan completes
- Added `IsIndexReady()` and `WaitForIndexReady(ctx)` methods for state queries
- Updated tests to use `WaitForIndexReady()` for synchronous behavior

**Phase 2: Template indexReady parameter threading**
- Removed superfluous `boolToString()` helper, using `strconv.FormatBool()` instead
- Added `indexReady bool` parameter to: `Layout`, `Home`, `HomeContent`, `Directory`, `Markdown`
- Added `data-index-ready` attribute to body tag
- Handlers now call `s.IsIndexReady()` and pass to all template calls

**Phase 3: Client-side SSE handling**
- Layout SSE handler now detects "index-ready" message, updates body attribute, dispatches custom event
- Command palette tracks `indexReady` state from body dataset
- Empty state shows "Indexing..." vs "No matches found" vs "Type to search"

**Phase 4: RecentlyUpdated component**
- Added `indexReady bool` parameter to `RecentlyUpdated` template
- Shows "Indexing..." state when scan not complete
- HomeContent tree display also shows "Indexing..." when not ready

Files modified:
- `internal/server/server.go` - Background scan infrastructure
- `internal/server/server_test.go` - Test synchronization
- `internal/server/handlers.go` - Pass indexReady to templates
- `web/templates/components/layout.templ` - SSE handling, removed boolToString
- `web/templates/components/command_palette.templ` - indexReady state and UI
- `web/templates/components/recent.templ` - indexReady parameter
- `web/templates/home.templ` - indexReady threading
- `web/templates/directory.templ` - indexReady parameter
- `web/templates/markdown.templ` - indexReady parameter
- `web/templates/templates_test.go` - Updated test calls

Verification: `make test && make lint && make build` - all pass (0 lint issues).

## 2025-12-20

Implemented "Toggle Gitignore" command for runtime control of .gitignore respect.

**Problem**: User running stigmergic in directory with `ai/` in .gitignore couldn't see markdown files in that directory. Previously required CLI flag `--respect-gitignore=false`.

**Solution**: Runtime-toggleable setting via command palette.

**Implementation**:
1. Added `respectGitignore atomic.Bool` to Server struct, initialized from config
2. `updateTree()` and `initialScan()` now read from atomic bool instead of config
3. Added `ToggleRespectGitignore()` method with atomic compare-and-swap, triggers rescan + SSE reload broadcast
4. Added `GET /api/gitignore` and `POST /api/gitignore/toggle` endpoints
5. Command palette now includes "Toggle Gitignore" command with dynamic description showing current state
6. Command fetches initial state on init, updates after each toggle

Session-only persistence: setting resets to config default on server restart.

Files modified:
- `internal/server/server.go` - Atomic bool field, toggle method, broadcast helper
- `internal/server/handlers.go` - Two new API endpoints, routes
- `web/templates/components/command_palette.templ` - Dynamic command with toggle action

Verification: `make test && make lint && make build` - all pass (0 lint issues).

## 2025-12-23

Added items to `ai/todo.md`, good simple features to add

## 2026-01-05

Implemented two features from `ai/todo.md`:

**Feature 1: Copy Relative Path Button**
- Added button in breadcrumb area to copy file path relative to `~/git/buildtall.systems`
- Server computes relative path via `computeBuildtallRelativePath()` function
- Button shows copy icon, switches to check icon (green) when copied
- Added `IconCopy` and `IconCheck` to icons.templ

**Feature 2: Momentary Line Numbers Button**
- Added "Lines" button that appears when viewing raw source
- Hold button to show line numbers, release to hide (pointerdown/pointerup/pointerleave handlers)
- Line numbers styled with theme's `--line-number-color` variable
- Raw content now rendered as table for proper line number alignment
- Content loaded via `templ.JSONScript` and parsed in Alpine.js `init()`

Files modified:
- `internal/server/handlers.go` - Added `computeBuildtallRelativePath()`, pass `relativePath` to templates
- `web/templates/markdown.templ` - Added `relativePath` parameter, copy button, line numbers table structure
- `web/templates/components/layout.templ` - Added `copyPath()` Alpine.js function, extended `rawToggle()` with `showLineNumbers` state and `loadRawContent()` method

Verification: `make generate && make lint && make build` - all pass (0 lint issues).

## 2026-02-11

Implemented optional NIP-98 Nostr authentication (GitHub issue #2).

**New package: `internal/auth/`**
- `nostr.go` - NIP-98 kind 27235 event verification, pubkey normalization (hex/npub), allowlist checking
- `session.go` - HMAC-signed stateless session cookies (`pubkey.expiryMillis.signature`), millisecond precision
- `middleware.go` - Auth middleware: passes through `/auth/*` and `/static/*`, redirects unauthenticated to login with redirect param, stores pubkey in request context
- `handlers.go` - LoginHandler (GET renders templ), VerifyHandler (POST validates NIP-98 + allowlist + sets cookie), LogoutHandler (POST clears cookie + redirects)
- Full test coverage: 33 tests across 4 test files, all passing with race detector

**New templates**
- `web/templates/login.templ` - Login page with NIP-07 browser extension integration (window.nostr)
- `web/templates/components/login_layout.templ` - Minimal standalone layout for login (no SSE/command palette)

**Config & CLI**
- `internal/config/config.go` - Added `AuthConfig` struct (enabled, allowed_npubs, session_secret, session_max_age)
- `cmd/stigmergic/serve.go` - Added `--auth` CLI flag override

**Integration**
- `internal/server/server.go` - Conditional auth middleware wrapping, pubkey normalization at startup (fail-fast), session manager initialization
- `internal/server/handlers.go` - Conditional auth route registration (`/auth/login`, `/auth/verify`, `/auth/logout`)

**Bug fix: watcher race condition (pre-existing)**
- `internal/watcher/watcher.go` - Added `debounceWg sync.WaitGroup` to track in-flight `time.AfterFunc` goroutines. `Close()` now properly stops timers (balancing WaitGroup for stopped timers) and waits for in-flight callbacks before closing channels. Eliminates race between `debounceEvent` timer goroutine and `Close()` channel teardown.

**Other lint fixes (pre-existing)**
- `internal/models/tree.go` - Extracted `MarkdownExt` constant for repeated `.md` string
- `internal/models/tree_test.go` - Replaced string literals with `MarkdownExt` constant

Verification: `make lint && make test && make build` — all pass (0 lint issues, 0 test failures, race-clean).

## 2026-02-28

Implemented wikilink backlinks (GitHub issue #9).

**New types: `internal/models/backlink.go`**
- `BacklinkEntry` struct (SourcePath, SourceTitle) and `BacklinkIndex` type alias — placed in models to avoid circular imports with templates

**New: `internal/markdown/backlinks.go`**
- `BuildBacklinkIndex(rootPath, files)` — builds parse-only goldmark instance with wikilink inline parser, walks every file's AST via `ast.Walk()`, resolves targets via existing `TreeResolver`, builds inverse index
- Self-links excluded, duplicate links from same source deduplicated
- Reuses `NewTreeResolver()` and `normalize()` from `wikilink.go`

**New: `internal/markdown/backlinks_test.go`**
- 5 test cases: multiple backlinks, no links, self-links, duplicate links, unresolved targets
- Uses `t.TempDir()` with on-disk markdown files

**Modified: `internal/server/server.go`**
- Added `cachedBacklinks atomic.Value` to Server struct
- Initialized with empty index in `NewServer()`
- Populated via `BuildBacklinkIndex()` in both `updateTree()` and `initialScan()` after file scanning

**Modified: `internal/server/handlers.go`**
- Loads backlinks from cached atomic, looks up entries for current file path
- Passes `[]models.BacklinkEntry` to both `Markdown()` and `MarkdownContent()` template calls

**Modified: `web/templates/markdown.templ`**
- Added `backlinks []models.BacklinkEntry` parameter to both template functions
- Conditional backlinks section after article using theme CSS variables (`--bg-alt-color`, `--border-color`, `--comment-color`, `--link-color`)

**Minor fix: `internal/server/handlers_test.go`**
- Removed stale `//nolint:gosec` directive

PR: https://github.com/Buildtall-Systems/stigmergic.dev/pull/11 (branch `feature/wikilink-backlinks` → `develop`)

Verification: `make generate && make lint && make test && make build` — all pass.

---

Released v0.2.0: https://github.com/Buildtall-Systems/stigmergic.dev/releases/tag/v0.2.0

Binaries: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64. Built via goreleaser (nix-shell). Merged develop → master, tagged, pushed.

## 2026-03-15

Fixed mermaid diagram rerendering after htmx live-reload swap (GitHub issue #16).

**Root cause**: `mermaid.initialize({ startOnLoad: true })` only fires on `DOMContentLoaded`. After an htmx swap, fresh `<pre class="mermaid">` elements from goldmark's client-side mermaid renderer are never processed.

**Fix**: Added `mermaid.run({ nodes })` call to the `htmx:afterSwap` handler in `layout.templ`, scoped to the swapped target element via `querySelectorAll('pre.mermaid')`. Only invokes when mermaid nodes are present.

Files modified:
- `web/templates/components/layout.templ` — 4 lines added to afterSwap handler

PR: https://github.com/Buildtall-Systems/stigmergic.dev/pull/17
Verification: `make generate && make build && make test` — all pass.

## 2026-07-03

Added n/p section jumping and reader mode (branch `feature/section-jump-keys`).

**Section jumping**: `n`/`p` leap the reading pane to the next/previous outline heading. Geometry-based against the scrollspy's heading list (`nav.js`), smooth scroll + outline highlight, standard modifier/input-target guards.

**Reader mode**: header Reader button or `r` stamps `[data-reader]` on `<html>`. CSS collapses both rails (without touching Alpine `sidebarOpen` state) and scales `.prose` to 2em — the 72ch cap widens with the font, so the column grows toward screen width at constant line measure while the header keeps its zoom. Persisted via localStorage, applied in `PrePaintScript` to avoid rail-flash on reload. Help overlay documents N/P/R.

**Toolchain fix required en route**: lint failed on `undefined: templ.ResolveAttributeValue` — devshell templ v0.3.943 (stale flake.lock) silently refused to regenerate templates (`updates=0`), go.mod pinned v0.3.960, stale artifacts stamped v0.3.1020. Bumped nixpkgs input (devshell templ now v0.3.1020), matched go.mod, resynced vendor/ (required: `vendorHash = null` consumes it).

Commits: `eefc58f` (toolchain), `e8deffc` (features). Verification: `make build && make lint && make test` in devshell — all pass. Operator verified both features in browser.

Remodeled the embedded site landing page (`site/content/index.md`): themed HTML hero with download CTAs, responsive feature-card grid, one-per-line keyboard shortcut list — all bare HTML blocks through the existing `WithUnsafe()` pipeline, styled with theme variables and em units so themes and reader mode compose. Stale screenshots removed (operator will retake); dropped the embedded-image assertion from `TestEmbeddedRealSiteFS` accordingly. Commit `9723e4a`. Verification: build, lint, tests all pass; operator reviewed in browser.

## 2026-08-04

Added j/k paragraph navigation (branch `feature/paragraph-nav`, worktree sibling).

**Paragraph jumping**: `j`/`k` scroll the reading pane to the next/previous top-level block of the rendered document (paragraphs, lists, code fences, tables, and headings alike), a finer-grained analog of the `n`/`p` section jumps. The shared geometry walk is extracted into `nearestBlock` in `nav.js`, used by both `jumpToSection` and the new `jumpToParagraph`. `documentBlocks` enumerates the article's direct children, filtering elements hidden by the source toggle. Handler `handleParagraphKeydown` mirrors the section handler's modifier and form-field guards; registered in `events.js`. Help overlay documents J/K.

Commits: `dd6e35a` (feature), `ad3afed` (help overlay). Verification: `make lint`, `make test`, `make build` in the worktree, all pass (worktree needed `pnpm install` first for the tailwind step). Operator browser verification pending; branch not yet integrated into develop.
