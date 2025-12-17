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
