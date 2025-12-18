# Plan: Embedded Website Mode for stigmergic.dev

**Goal**: App serves its own documentation from content embedded in the binary.

## Architecture

Introduce a `ContentSource` interface that abstracts filesystem operations. Both filesystem mode (`serve`) and embedded mode (`site`) implement this interface.

```go
ContentSource interface {
    BuildTree() (*models.Tree, error)
    ReadFile(path string) ([]byte, error)
    Stat(path string) (fs.FileInfo, error)
    IsEmbedded() bool
}
```

## Implementation Phases

### Phase 1: Content Source Abstraction

**Create `internal/source/source.go`**
- Define `ContentSource` interface

**Create `internal/source/filesystem.go`**
- `FilesystemSource` struct wrapping existing `os.*` calls
- `BuildTree()` delegates to existing `watcher.ScanDirectory()`
- `ReadFile()` wraps `os.ReadFile()`
- `Stat()` wraps `os.Stat()`
- `IsEmbedded()` returns `false`

### Phase 2: Embed Site Content

**Create `site/` directory** with initial documentation:
- `site/index.md` - Landing page
- `site/getting-started.md` - Installation and usage
- `site/features/` - Document key features

**Create `internal/embed/site.go`**
```go
//go:embed site
var siteFiles embed.FS
```

**Create `internal/source/embedded.go`**
- `EmbeddedSource` struct wrapping `embed.FS`
- `BuildTree()` uses `fs.WalkDir()` to build tree from embedded FS
- `ReadFile()` uses `fs.ReadFile()`
- `Stat()` uses `fs.Stat()`
- `IsEmbedded()` returns `true`

### Phase 3: Server Modifications

**Modify `internal/server/server.go`**
- Add `source source.ContentSource` field to `Server` struct
- Create `NewServerWithSource(cfg, src)` constructor
- Conditional watcher setup: skip when `src.IsEmbedded()`
- `updateTree()` uses `src.BuildTree()`

**Modify `internal/server/handlers.go`**
- `handleMarkdown()`: Replace `os.Stat(cleanPath)` with `s.source.Stat(filePath)`
- `handleMarkdown()`: Replace `os.ReadFile(cleanPath)` with `s.source.ReadFile(filePath)`
- Path validation: work with relative paths from source root
- `handleSSE()`: Return no-op for embedded mode (no live reload)

### Phase 4: Site Subcommand

**Create `cmd/stigmergic/site.go`**
- `site` subcommand (no path argument needed)
- Creates `EmbeddedSource`
- Calls `server.NewServerWithSource(cfg, src)`
- Port negotiation same as `serve`

**Modify `cmd/stigmergic/main.go`**
- Register `siteCmd`

### Phase 5: Tests

**Create `internal/source/source_test.go`**
- Test `FilesystemSource` with temp directory
- Test `EmbeddedSource` tree building

**Modify `internal/server/server_test.go`**
- Test server with embedded source
- Verify watcher is nil in embedded mode

## Critical Files

| File | Action |
|------|--------|
| `internal/source/source.go` | Create |
| `internal/source/filesystem.go` | Create |
| `internal/source/embedded.go` | Create |
| `internal/embed/site.go` | Create |
| `internal/server/server.go` | Modify lines 21-99 |
| `internal/server/handlers.go` | Modify lines 62-148, 170-218 |
| `cmd/stigmergic/site.go` | Create |
| `cmd/stigmergic/main.go` | Modify (add siteCmd) |
| `site/*.md` | Create |

## Key Implementation Details

### Path Handling
- `embed.FS` uses forward slashes regardless of OS
- Relative paths throughout for embedded mode
- `Tree.Find()` needs to work with relative paths (currently uses absolute)

### SSE in Embedded Mode
```go
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
    if s.source.IsEmbedded() {
        w.Header().Set("Content-Type", "text/event-stream")
        w.Write([]byte(": embedded mode\n\n"))
        return
    }
    // ... existing implementation
}
```

### Tree.Find() Adaptation
Currently `tree.Find(cleanPath)` uses absolute path (`/home/user/docs/file.md`).
For embedded mode, needs to work with relative paths (`docs/file.md`).

Option: Store relative paths in tree nodes, let `Find()` accept relative paths.

## Execution Order

1. Create `internal/source/` package with interface and filesystem implementation
2. Verify existing functionality works unchanged via `FilesystemSource`
3. Create `site/` directory with placeholder docs
4. Create embedded source implementation
5. Modify server to accept `ContentSource`
6. Add `site` subcommand
7. Test full integration
8. Write remaining documentation content

## Success Criteria

- `stigmergic site` serves embedded docs on localhost
- `stigmergic serve [path]` works unchanged
- No live reload in embedded mode (expected)
- File tree navigation works with embedded content
- All existing tests pass + new tests for source abstraction
