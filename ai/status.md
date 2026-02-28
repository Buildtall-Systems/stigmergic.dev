# Status: stigmergic.dev

Daily work log. Add entries under date headers (## YYYY-MM-DD) after each unit of work.

See `docs/operations/status-spec.md` for format specification.

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
