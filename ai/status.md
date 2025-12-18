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
