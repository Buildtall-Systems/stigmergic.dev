# Linter Issues Catalog

Captured 2026-02-24 after migrating `.golangci.yml` to v2 syntax and removing the `--max-issues-per-linter=1` cap from the Makefile that was masking issues.

52 issues total: errcheck (25), govet (15), nolintlint (12).

## Plan

### nolintlint (12 issues) — Remove unused directives

The `.golangci.yml` v2 exclusion rules now handle `os.WriteFile` permissions in test files, making per-line `//nolint:gosec` directives redundant. Remove them all.

**Files:**
- `internal/server/handlers_test.go` — lines 314, 406
- `internal/watcher/scanner_test.go` — line 131
- `internal/watcher/watcher_test.go` — lines 78, 160, 177, 202, 219, 261, 286, 340, 391

### errcheck: check-blank (17 issues) — Proper error handling for `_ = fn()`

The `check-blank: true` setting flags `_ = fn()` as unchecked. These need case-by-case fixes:

**Goldmark renderer writes (11 issues)** — `w.WriteString` / `w.Write` in renderers write to `bytes.Buffer` which cannot fail. However, the renderer interface returns `ast.WalkStatus` not `error`, so we cannot propagate. The goldmark convention is to ignore these. Fix: disable `check-blank` for errcheck (the `_ =` pattern IS the explicit acknowledgment) and rely on `check-type-assertions` for real issues.

Actually: `check-blank: true` is overly aggressive. The Go idiom `_ = fn()` explicitly means "I acknowledge this error and intentionally discard it." That is the opposite of an unchecked error. Set `check-blank: false`.

- `internal/markdown/extensions.go` — lines 171, 174, 176, 183, 186, 188
- `internal/markdown/wikilink.go` — lines 142, 143, 144, 146, 154, 156

**Resource cleanup in defers (3 issues)** — `_ = listener.Close()`, `_ = file.Close()`. Errors on close in defers are conventionally discarded. Same resolution: `check-blank: false`.

- `cmd/stigmergic/serve.go` — line 20
- `internal/watcher/scanner.go` — line 36
- `internal/watcher/watcher.go` — line 107

**Testutil (2 issues)** — `_ = listener.Close()` and type assertion in test helpers. Same resolution.

- `internal/testutil/testutil.go` — lines 35, 37

### errcheck: type assertions (4 issues) — Fix properly

These use bare type assertions without `ok` check and should be fixed with comma-ok idiom.

- `internal/auth/middleware.go:43` — `v, _ := ctx.Value(pubkeyContextKey).(string)` — use comma-ok
- `internal/markdown/extensions.go:185` — `segment := c.(*ast.Text).Segment` — use comma-ok with guard
- `internal/server/handlers.go:66,128,277` — `s.cachedFiles.Load().([]models.SearchableFile)` — use comma-ok with guard

### errcheck: unchecked return values (4 issues) — Fix properly

- `internal/models/tree.go:44` — `absPath, _ := filepath.Abs(rootPath)` — check error
- `internal/models/tree.go:86` — `absPath, _ := filepath.Abs(path)` — check error
- `internal/watcher/watcher.go:131` — `relPath, _ := filepath.Rel(w.rootPath, path)` — check error

### govet: fieldalignment (10 issues) — Reorder struct fields

Reorder fields by decreasing alignment size (pointers/interfaces first, then int64, then int32, then smaller types).

- `internal/auth/handlers.go:17` — `verifyResponse`
- `internal/config/config.go:11` — `AuthConfig`
- `internal/config/config.go:18` — `Config`
- `internal/markdown/wikilink.go:97` — `WikilinkRenderer`
- `internal/models/tree.go:23` — `SearchableFile`
- `internal/models/tree.go:30` — `Node`
- `internal/server/server.go:23` — `Server`
- `internal/watcher/watcher.go:28` — `Event`
- `internal/watcher/watcher.go:33` — `Watcher`

### govet: shadow (3 issues) — Rename shadowed variables

- `internal/server/handlers.go:145,150` — `err` shadows declaration at line 117. Use `renderErr` or hoist.
- `internal/server/server.go:95` — `err` shadows declaration at line 88. Use `addErr` or restructure.

### govet: unusedwrite (1 issue, 3 fields) — Fix test

- `internal/models/tree_test.go:193-195` — struct fields written but never read. Fix the test to actually assert on the values.

## Execution Order

1. Config: set `check-blank: false` in `.golangci.yml` errcheck settings (eliminates 17 errcheck issues)
2. Remove unused `//nolint:gosec` directives from test files (eliminates 12 nolintlint issues)
3. Fix type assertions with comma-ok idiom (4 errcheck issues)
4. Fix unchecked `filepath.Abs` and `filepath.Rel` return values (3 errcheck issues)
5. Reorder struct fields for alignment (10 govet issues)
6. Fix variable shadowing (3 govet issues)
7. Fix unusedwrite in test (1 govet issue, 3 fields)
8. Run `make lint` — must pass clean
9. Run `make test` — must pass
