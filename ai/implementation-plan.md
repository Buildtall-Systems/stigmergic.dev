# stigmergic.dev - Implementation Plan

## Overview

This document outlines a strategic, iterative approach to implementing stigmergic.dev. The plan follows a bottom-up methodology, establishing infrastructure first, then building core functionality, and finally polishing the user experience.

## Phase 0: Project Foundation & Infrastructure

### Step 0.1: Nix Development Environment
**Goal**: Create a reproducible development environment

**Tasks**:
1. Create `flake.nix` with all required dependencies:
   - Go (latest stable)
   - gopls (Go language server)
   - gotools (Go development tools)
   - templ CLI
   - tailwindcss CLI
   - nodejs (for Tailwind build process)
2. Define `devShells.default` for `nix develop`
3. Test environment by entering shell and verifying all tools

**Success Criteria**:
- `nix develop` successfully enters dev environment
- All tools available: `go version`, `templ version`, `tailwindcss --help`

### Step 0.2: Go Module Initialization
**Goal**: Initialize Go module and dependency management

**Tasks**:
1. Run `go mod init github.com/YOUR_USERNAME/stigmergic`
2. Create basic directory structure:
   ```
   stigmergic/
   ├── cmd/stigmergic/
   ├── internal/
   │   ├── server/
   │   ├── watcher/
   │   ├── markdown/
   │   ├── config/
   │   └── models/
   ├── web/
   │   ├── templates/
   │   └── static/
   ├── ai/
   ├── flake.nix
   └── go.mod
   ```
3. Create placeholder `.keep` files in empty directories

**Success Criteria**:
- `go.mod` exists with correct module path
- Directory structure matches architecture

### Step 0.3: Add Go Dependencies
**Goal**: Add all required Go dependencies to go.mod

**Tasks**:
1. Add dependencies:
   ```bash
   go get github.com/spf13/cobra@latest
   go get github.com/spf13/viper@latest
   go get github.com/yuin/goldmark@latest
   go get github.com/alecthomas/chroma/v2@latest
   go get github.com/fsnotify/fsnotify@latest
   go get github.com/a-h/templ@latest
   ```
2. Run `go mod tidy`
3. Update `flake.nix` with `vendorHash` after running `go mod vendor`

**Success Criteria**:
- All dependencies in `go.mod`
- `go mod verify` succeeds
- No errors from `go mod tidy`

### Step 0.4: Testing Infrastructure
**Goal**: Set up testing framework and ensure test isolation

**Tasks**:
1. Create `internal/testutil/testutil.go` with helper functions:
   - `CreateTempDir(t *testing.T) string` - creates isolated temp dir with `t.TempDir()`
   - `CreateTestFile(t *testing.T, dir, name, content string)` - creates test fixtures
   - `FindAvailablePort(t *testing.T) int` - finds random available port
2. Create example test file `internal/testutil/testutil_test.go`
3. Run `go test ./...` to verify

**Success Criteria**:
- Test utilities available for all packages
- `go test ./...` passes
- Tests can be run with `go test -parallel 10` safely

## Phase 1: CLI Foundation (Cobra + Viper)

### Step 1.1: Basic CLI Structure
**Goal**: Create minimal cobra CLI that can be invoked

**Tasks**:
1. Gather context for cobra: reference `/spf13/cobra` from context7
2. Create `cmd/stigmergic/main.go` with cobra root command
3. Create `cmd/stigmergic/root.go` with basic command structure
4. Add `--version` flag
5. Build and test: `go build -o stigmergic ./cmd/stigmergic`

**Success Criteria**:
- `./stigmergic --help` shows help text
- `./stigmergic --version` shows version
- Binary compiles without errors

### Step 1.2: Configuration with Viper
**Goal**: Implement configuration loading from file, env, and flags

**Tasks**:
1. Gather context for viper: reference `/spf13/viper` from context7
2. Create `internal/config/config.go`:
   - Define `Config` struct with all settings
   - Implement `Load()` function using viper
3. Create `internal/config/config_test.go`:
   - Test config file loading (TOML)
   - Test environment variable override
   - Test flag precedence
   - Use isolated temp directories for each test
4. Add config flags to root command:
   - `--port` / `-p` (default: 8080)
   - `--host` / `-h` (default: localhost)
   - `--config` / `-c` (default: .stigmergic.toml)
5. Bind viper to cobra flags

**Success Criteria**:
- Config loads from `.stigmergic.toml`
- Environment variables override config file (STIGMERGIC_PORT, etc.)
- CLI flags override everything
- All config tests pass with proper isolation

### Step 1.3: Serve Command
**Goal**: Create the main `serve` command that accepts a path argument

**Tasks**:
1. Create `cmd/stigmergic/serve.go` with serve command
2. Accept path as positional argument (default: current directory)
3. Validate path exists and is a directory
4. Wire up config to serve command
5. Add basic logging of configuration

**Success Criteria**:
- `./stigmergic serve /path/to/docs` validates path
- `./stigmergic serve` uses current directory
- Invalid paths show helpful error messages
- Configuration values are logged at startup

## Phase 2: File Tree & Watcher

### Step 2.1: File Tree Model
**Goal**: Create data structures for representing directory trees

**Tasks**:
1. Create `internal/models/tree.go`:
   - Define `Node` struct (path, name, isDir, children)
   - Define `Tree` struct (root node, index map)
2. Create `internal/models/tree_test.go`:
   - Test tree creation from directory
   - Test tree traversal
   - Use temp directories for test fixtures

**Success Criteria**:
- Tree can represent nested directories
- Tree can be serialized/traversed efficiently
- All tests pass with isolation

### Step 2.2: File Tree Scanner
**Goal**: Build initial file tree from filesystem

**Tasks**:
1. Create `internal/watcher/scanner.go`:
   - Implement `ScanDirectory(path string) (*models.Tree, error)`
   - Walk directory structure
   - Filter for markdown files and directories
   - Build tree model
2. Create `internal/watcher/scanner_test.go`:
   - Test scanning directory with markdown files
   - Test nested directory scanning
   - Test empty directory handling
   - Use `testutil.CreateTempDir` and `testutil.CreateTestFile`

**Success Criteria**:
- Can scan directory and build tree
- Performance: < 500ms for 1000 files
- All tests pass with proper isolation

### Step 2.3: Filesystem Watcher
**Goal**: Implement real-time file watching with fsnotify

**Tasks**:
1. Gather context for fsnotify: reference `/fsnotify/fsnotify` from context7
2. Create `internal/watcher/watcher.go`:
   - Define `Watcher` struct with fsnotify watcher
   - Implement `Watch(path string) error`
   - Implement `Events() <-chan Event`
   - Handle create, modify, delete, rename events
   - Debounce rapid events (200ms window)
3. Create `internal/watcher/watcher_test.go`:
   - Test file creation detection
   - Test file modification detection
   - Test file deletion detection
   - Use isolated temp directories
   - Use `t.Cleanup()` to stop watchers

**Success Criteria**:
- Watcher detects file system changes
- Events properly debounced
- No leaked goroutines after tests
- Performance: < 200ms from file change to event

## Phase 3: HTTP Server Foundation

### Step 3.1: Basic HTTP Server
**Goal**: Create minimal HTTP server with graceful shutdown

**Tasks**:
1. Create `internal/server/server.go`:
   - Define `Server` struct
   - Implement `NewServer(config *config.Config) *Server`
   - Implement `Start() error` and `Shutdown(ctx context.Context) error`
   - Add graceful shutdown on SIGINT/SIGTERM
2. Create `internal/server/server_test.go`:
   - Test server start/stop
   - Test graceful shutdown
   - Use `testutil.FindAvailablePort()`
   - Ensure cleanup with `t.Cleanup()`

**Success Criteria**:
- Server starts on configured port
- Server responds to requests
- Graceful shutdown works
- Tests don't leak servers

### Step 3.2: Static File Serving
**Goal**: Serve static assets (CSS, JS)

**Tasks**:
1. Create `web/static/js/` directory and add htmx
2. Create `web/static/styles/` directory
3. Add static file handler in `internal/server/handlers.go`
4. Embed static files using `//go:embed` directive for self-contained binary
5. Create test to verify static files served correctly

**Success Criteria**:
- Static files accessible at `/static/*`
- Correct MIME types set
- 404 for missing files
- Static files embedded in binary (works from any directory)

### Step 3.3: Middleware
**Goal**: Add logging and basic middleware

**Tasks**:
1. Create `internal/server/middleware.go`:
   - Request logging middleware
   - Recovery middleware (panic handler)
   - Security headers middleware
2. Add middleware to server
3. Test middleware behavior

**Success Criteria**:
- All requests logged
- Panics caught and logged
- Security headers present (X-Content-Type-Options, etc.)

## Phase 4: Templating with Templ

### Step 4.1: Templ Setup
**Goal**: Get templ working with build process

**Tasks**:
1. Gather context for templ: reference `/a-h/templ` from context7
2. Create `web/templates/base.templ` with HTML5 boilerplate
3. Add templ generation to build process
4. Create `Makefile` with:
   - `make generate` - runs `templ generate`
   - `make build` - runs generate + go build
   - `make test` - runs tests
5. Update `.gitignore` to ignore generated `*_templ.go` files

**Success Criteria**:
- `templ generate` creates Go code
- Templates compile successfully
- Generated code in gitignore

### Step 4.2: Base Layout
**Goal**: Create base layout template with navigation

**Tasks**:
1. Create `web/templates/components/layout.templ`:
   - HTML5 structure
   - Head with meta tags
   - Link to Tailwind CSS
   - Link to htmx
   - Main content area
2. Create basic styling structure
3. Test template renders

**Success Criteria**:
- Base layout renders valid HTML5
- CSS and JS properly linked
- Responsive meta tags present

### Step 4.3: Home Page Template
**Goal**: Create home page showing file tree

**Tasks**:
1. Create `web/templates/home.templ`:
   - Uses base layout
   - Accepts tree model
   - Renders tree as nested list
   - Links to markdown files
2. Create `internal/server/handlers.go` home handler:
   - Gets tree from watcher
   - Renders home template
3. Wire up route: `GET /`

**Success Criteria**:
- Home page shows file tree
- Tree structure visually clear
- Links to files work

## Phase 5: Markdown Rendering

### Step 5.1: Basic Goldmark Setup
**Goal**: Render markdown to HTML with goldmark

**Tasks**:
1. Gather context for goldmark: reference `/yuin/goldmark` from context7
2. Create `internal/markdown/parser.go`:
   - Initialize goldmark parser with CommonMark compliance
   - Implement `Parse([]byte) ([]byte, error)` - converts MD to HTML
3. Create `internal/markdown/parser_test.go`:
   - Test basic markdown rendering
   - Test headings, lists, links, etc.

**Success Criteria**:
- Markdown converts to HTML correctly
- CommonMark compliant
- All tests pass

### Step 5.2: Syntax Highlighting (Chroma)
**Goal**: Add code syntax highlighting

**Tasks**:
1. Gather context for chroma: reference `/alecthomas/chroma` from context7
2. Update `internal/markdown/parser.go`:
   - Add chroma goldmark extension
   - Configure highlighting style
3. Create `internal/markdown/extensions.go` for extension setup
4. Test with various code languages

**Success Criteria**:
- Code blocks have syntax highlighting
- Multiple languages supported
- Highlighting works without JavaScript

### Step 5.3: GFM Extensions
**Goal**: Add GitHub Flavored Markdown features

**Tasks**:
1. Update `internal/markdown/parser.go`:
   - Add GFM extension for tables
   - Add GFM extension for strikethrough
   - Add GFM extension for task lists
2. Test each GFM feature

**Success Criteria**:
- Tables render correctly
- Strikethrough works
- Task lists render with checkboxes

### Step 5.4: Math Rendering (KaTeX)
**Goal**: Render LaTeX math formulas

**Tasks**:
1. Gather context for katex: reference `/katex/katex` from context7
2. Add KaTeX CSS and JS to base template
3. Create custom goldmark extension for math:
   - Detect `$...$` inline math
   - Detect `$$...$$` block math
   - Add custom AST nodes
4. Test math rendering

**Success Criteria**:
- Inline math renders correctly
- Block math renders correctly
- KaTeX loads from CDN or local

### Step 5.5: Mermaid Diagrams
**Goal**: Render Mermaid diagrams

**Tasks**:
1. Gather context for mermaid: reference `/mermaid-js/mermaid` from context7
2. Add Mermaid JS to base template
3. Create custom goldmark extension:
   - Detect ```mermaid code blocks
   - Mark with special class for Mermaid
   - Add initialization script
4. Test various diagram types

**Success Criteria**:
- Flowcharts render
- Sequence diagrams render
- ER diagrams render
- All Mermaid types work

### Step 5.6: Nostr URL Parsing
**Goal**: Parse and link nostr:// URLs

**Tasks**:
1. Create custom goldmark extension in `internal/markdown/extensions.go`:
   - Regex to match nostr:npub, nostr:note, nostr:nevent
   - Convert to clickable links
   - Add appropriate href format
2. Test nostr URL detection and linking

**Success Criteria**:
- Nostr URLs detected in markdown
- URLs converted to links
- Links have proper format

### Step 5.7: Markdown View Template
**Goal**: Create template for rendered markdown

**Tasks**:
1. Create `web/templates/markdown.templ`:
   - Uses base layout
   - Displays breadcrumb navigation
   - Renders markdown HTML
   - Styled content area
2. Create handler for markdown files:
   - Read file from disk
   - Parse with goldmark
   - Render template
3. Wire up route: `GET /file/*path`

**Success Criteria**:
- Markdown files render beautifully
- All extensions work
- Breadcrumb navigation present
- Back button works

## Phase 6: Tailwind CSS & Styling

### Step 6.1: Tailwind Setup
**Goal**: Configure Tailwind CSS build process

**Tasks**:
1. Gather context for tailwindcss: reference `/websites/tailwindcss` from context7
2. Create `tailwind.config.js`:
   - Configure content paths for templates
   - Set up theme customization
3. Create `web/static/styles/input.css` with Tailwind directives
4. Add build script to generate CSS
5. Update Makefile with CSS build

**Success Criteria**:
- Tailwind builds successfully
- Generated CSS in static directory
- Utility classes work in templates

### Step 6.2: Typography & Layout Styling
**Goal**: Style markdown content beautifully

**Tasks**:
1. Add Tailwind Typography plugin
2. Style markdown content area:
   - Proper font hierarchy
   - Comfortable line height and spacing
   - Code block styling
   - Table styling
3. Style navigation and tree view
4. Add responsive design

**Success Criteria**:
- Content is readable and beautiful
- Typography follows best practices
- Works on mobile and desktop

### Step 6.3: Component Styling
**Goal**: Polish all UI components

**Tasks**:
1. Style file tree navigation
2. Style breadcrumbs
3. Style buttons and links
4. Add hover states and transitions
5. Ensure consistent spacing

**Success Criteria**:
- Consistent visual design
- Professional appearance
- Smooth interactions

## Phase 7: HTMX Integration

### Step 7.1: HTMX Setup
**Goal**: Add HTMX for dynamic updates

**Tasks**:
1. Gather context for htmx: reference `/bigskysoftware/htmx` from context7
2. Ensure htmx.js loaded in base template
3. Configure htmx defaults
4. Add HTMX attributes to navigation links

**Success Criteria**:
- HTMX loads without errors
- Links use HTMX for navigation
- No full page refreshes

### Step 7.2: Partial Template Updates
**Goal**: Return HTML fragments for HTMX

**Tasks**:
1. Create `web/templates/components/content.templ` for main content area
2. Update handlers to detect HTMX requests
3. Return full page or partial based on request
4. Add proper HTMX response headers

**Success Criteria**:
- Navigation updates content without refresh
- Browser back button works
- URL updates correctly

### Step 7.3: Live Reload with SSE
**Goal**: Auto-reload content when files change

**Tasks**:
1. Create SSE endpoint: `GET /events`
2. Connect watcher events to SSE stream
3. Add HTMX extension for SSE
4. Auto-refresh content on file change
5. Show notification on update

**Success Criteria**:
- Content updates when file changes
- < 1 second from change to update
- Visual feedback for updates

## Phase 8: Integration & Polish

### Step 8.1: Full Integration Testing
**Goal**: Test entire workflow end-to-end

**Tasks**:
1. Create integration tests:
   - Start server
   - Make HTTP requests
   - Verify responses
   - Test watcher integration
2. Test all markdown extensions together
3. Test configuration scenarios

**Success Criteria**:
- All integration tests pass
- No race conditions
- Proper resource cleanup

### Step 8.2: Error Handling
**Goal**: Graceful error handling throughout

**Tasks**:
1. Add error templates
2. Handle missing files gracefully
3. Handle invalid markdown
4. Handle configuration errors
5. Add helpful error messages

**Success Criteria**:
- No panics in production
- User-friendly error messages
- Proper HTTP status codes

### Step 8.3: Performance Optimization
**Goal**: Meet performance targets

**Tasks**:
1. Profile with pprof
2. Optimize hot paths
3. Add caching where appropriate
4. Optimize file scanning for large directories

**Success Criteria**:
- Server startup < 100ms
- Tree generation < 500ms for 1000 files
- Markdown rendering < 50ms per file
- Memory usage < 50MB

### Step 8.4: Documentation
**Goal**: Complete user and developer documentation

**Tasks**:
1. Write README.md:
   - Installation instructions
   - Usage examples
   - Configuration guide
2. Write CONTRIBUTING.md
3. Add code documentation/comments (only where necessary for API docs)
4. Create example .stigmergic.toml

**Success Criteria**:
- Clear installation instructions
- Examples for common use cases
- Developer setup guide

## Phase 9: Nix Packaging

### Step 9.1: Complete Nix Package
**Goal**: Make installable via Nix

**Tasks**:
1. Update `flake.nix`:
   - Complete package definition
   - Calculate correct vendorHash
   - Add metadata (description, license)
2. Test installation: `nix profile install .#`
3. Test direct run: `nix run .# -- /path/to/docs`

**Success Criteria**:
- Installs successfully with Nix
- Binary works after install
- Can run directly from GitHub

### Step 9.2: CI/CD with Nix
**Goal**: Automated builds and tests

**Tasks**:
1. Create `.github/workflows/ci.yml`:
   - Build with Nix
   - Run tests
   - Check formatting
2. Create `.github/workflows/release.yml`:
   - Build binaries for multiple platforms
   - Create GitHub releases

**Success Criteria**:
- CI runs on every push
- Tests run in Nix environment
- Releases automated

## Phase 10: Testing & Release

### Step 10.1: Comprehensive Testing
**Goal**: Full test coverage

**Tasks**:
1. Run full test suite with coverage
2. Add missing tests
3. Test on Linux and macOS
4. Manual testing of all features
5. Fix any bugs found

**Success Criteria**:
- Test coverage > 80%
- All tests pass
- No known bugs

### Step 10.2: Beta Release
**Goal**: v0.1.0-beta release

**Tasks**:
1. Tag version v0.1.0-beta
2. Build release binaries
3. Create GitHub release
4. Share with beta testers

**Success Criteria**:
- Release artifacts available
- Installation works
- Beta feedback collected

### Step 10.3: v1.0.0 Release
**Goal**: Production-ready release

**Tasks**:
1. Address beta feedback
2. Final polish and testing
3. Tag v1.0.0
4. Create release announcement
5. Update documentation

**Success Criteria**:
- Meets all success criteria from spec
- Stable and reliable
- Ready for production use

## Milestone Tracking

Following the milestones from spec.md:

- **M1 - Foundation**: Phases 0-1 (Infrastructure + CLI)
- **M2 - Navigation**: Phases 2-3 (File tree + HTTP server)
- **M3 - Extensions**: Phase 5 (Markdown rendering)
- **M4 - Watching**: Phase 2.3 + 7.3 (File watching + Live reload)
- **M5 - Configuration**: Phase 1.2 (Viper configuration)
- **M6 - Polish**: Phases 6 + 8 (Styling + Integration)
- **M7 - Nix**: Phase 9 (Nix packaging)
- **M8 - Release**: Phase 10 (Testing + Release)

## Notes

- **Context First**: Always gather context from ai/context.md before using any library
- **Test Isolation**: Every test must use isolated temp directories and ports
- **Iterative**: Each phase builds on previous phases
- **Testable**: Each step has clear success criteria
- **No Comments**: Remember the NO COMMENTS rule from CLAUDE.md
