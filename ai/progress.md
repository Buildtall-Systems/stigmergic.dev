what is this
# Project Progress

**Project:** stigmergic.dev
**Type:** project

## Progress Summary

**Total Steps**: 43
**Completed**: 28
**In Progress**: 0
**Remaining**: 15

**Current Phase**: Phase 7 - HTMX Integration
**Current Step**: Complete

---

## Phase 0: Project Foundation & Infrastructure

- [x] **Step 0.1: Nix Development Environment**
  - [x] Create `flake.nix` with all required dependencies
  - [x] Define `devShells.default` for `nix develop`
  - [x] Test environment by entering shell and verifying all tools

- [x] **Step 0.2: Go Module Initialization**
  - [x] Run `go mod init github.com/Buildtall-Systems/stigmergic.dev`
  - [x] Create basic directory structure
  - [x] Create placeholder `.keep` files in empty directories

- [x] **Step 0.3: Add Go Dependencies**
  - [x] Add cobra dependency
  - [x] Add viper dependency
  - [x] Add goldmark dependency
  - [x] Add chroma dependency
  - [x] Add fsnotify dependency (auto-added via viper)
  - [x] Add templ dependency
  - [x] Run `go mod tidy`
  - [x] vendorHash set to null (Nix will calculate it during build)

- [x] **Step 0.4: Testing Infrastructure**
  - [x] Create `internal/testutil/testutil.go` with helper functions
  - [x] Create example test file `internal/testutil/testutil_test.go`
  - [x] Run `go test ./...` to verify

---

## Phase 1: CLI Foundation (Cobra + Viper)

- [x] **Step 1.1: Basic CLI Structure**
  - [x] Gather context for cobra from `/spf13/cobra`
  - [x] Create `cmd/stigmergic/main.go` with cobra root command
  - [x] Root command structure in main.go (no separate root.go needed)
  - [x] Add `--version` flag
  - [x] Build and test: `go build -o stigmergic ./cmd/stigmergic`

- [x] **Step 1.2: Configuration with Viper**
  - [x] Gather context for viper from `/spf13/viper`
  - [x] Create `internal/config/config.go` with Config struct and Load function
  - [x] Create `internal/config/config_test.go` with comprehensive tests
  - [x] Add config flags to root command
  - [x] Bind viper to cobra flags

- [x] **Step 1.3: Serve Command**
  - [x] Create `cmd/stigmergic/serve.go` with serve command
  - [x] Accept path as positional argument
  - [x] Validate path exists and is a directory
  - [x] Wire up config to serve command
  - [x] Add basic logging of configuration

---

## Phase 2: File Tree & Watcher

- [x] **Step 2.1: File Tree Model**
  - [x] Create `internal/models/tree.go` with Node and Tree structs
  - [x] Create `internal/models/tree_test.go` with comprehensive tests
  - [x] **BUG FIX COMPLETED**: Node.Path now stores relative paths (root=".", children relative to root)
  - [x] Added RootPath field to Tree struct to store absolute root path

- [x] **Step 2.2: File Tree Scanner**
  - [x] Create `internal/watcher/scanner.go` with ScanDirectory function
  - [x] Create `internal/watcher/scanner_test.go` with comprehensive tests
  - [x] **BUG FIX COMPLETED**: Scanner now stores relative paths instead of absolute paths

- [x] **Step 2.3: Filesystem Watcher**
  - [x] Gather context for fsnotify from `/fsnotify/fsnotify`
  - [x] Create `internal/watcher/watcher.go` with Watcher struct
  - [x] Create `internal/watcher/watcher_test.go` with comprehensive tests

---

## Phase 3: HTTP Server Foundation

- [x] **Step 3.1: Basic HTTP Server**
  - [x] Create `internal/server/server.go` with Server struct
  - [x] Create `internal/server/server_test.go` with comprehensive tests

- [x] **Step 3.2: Static File Serving**
  - [x] Create `web/static/js/` directory and add htmx
  - [x] Create `web/static/styles/` directory
  - [x] Add static file handler in `internal/server/handlers.go`
  - [x] Create test to verify static files served correctly

- [x] **Step 3.3: Middleware**
  - [x] Create `internal/server/middleware.go` with logging, recovery, and security middleware
  - [x] Add middleware to server
  - [x] Test middleware behavior

---

## Phase 4: Templating with Templ

- [x] **Step 4.1: Templ Setup**
  - [x] Gather context for templ from `/a-h/templ`
  - [x] Create `web/templates/base.templ` with HTML5 boilerplate
  - [x] Add templ generation to build process
  - [x] Create `Makefile` with generate, build, and test targets
  - [x] Update `.gitignore` to ignore generated `*_templ.go` files

- [x] **Step 4.2: Base Layout**
  - [x] Create `web/templates/components/layout.templ` with base HTML structure
  - [x] Create basic styling structure
  - [x] Test template renders

- [x] **Step 4.3: Home Page Template**
  - [x] Create `web/templates/home.templ` with file tree rendering
  - [x] Create `internal/server/handlers.go` home handler
  - [x] Wire up route: `GET /`

---

## Phase 5: Markdown Rendering

- [x] **Step 5.1: Basic Goldmark Setup**
  - [x] Gather context for goldmark from `/yuin/goldmark`
  - [x] Create `internal/markdown/parser.go` with Parse function
  - [x] Create `internal/markdown/parser_test.go` with comprehensive tests

- [x] **Step 5.2: Syntax Highlighting (Chroma)**
  - [x] Gather context for chroma from `/alecthomas/chroma`
  - [x] Update `internal/markdown/parser.go` with chroma extension
  - [x] Create `internal/markdown/extensions.go` for extension setup
  - [x] Test with various code languages

- [x] **Step 5.3: GFM Extensions**
  - [x] Add GFM extension for tables
  - [x] Add GFM extension for strikethrough
  - [x] Add GFM extension for task lists
  - [x] Test each GFM feature

- [x] **Step 5.4: Math Rendering (KaTeX)**
  - [x] Gather context for katex from `/katex/katex`
  - [x] Add KaTeX CSS and JS to base template
  - [x] Create custom goldmark extension for math
  - [x] Test math rendering

- [x] **Step 5.5: Mermaid Diagrams**
  - [x] Gather context for mermaid from `/mermaid-js/mermaid`
  - [x] Add Mermaid JS to base template
  - [x] Create custom goldmark extension for mermaid blocks
  - [x] Test various diagram types

- [x] **Step 5.6: Nostr URL Parsing**
  - [x] Create custom goldmark extension for nostr URLs
  - [x] Test nostr URL detection and linking

- [x] **Step 5.7: Markdown View Template**
  - [x] Create `web/templates/markdown.templ` with markdown rendering
  - [x] Create handler for markdown files
  - [x] Wire up route: `GET /file/*path`

---

## Phase 6: Tailwind CSS & Styling

- [x] **Step 6.1: Tailwind Setup**
  - [x] Gather context for tailwindcss from `/websites/tailwindcss`
  - [x] Create `tailwind.config.js` with content paths
  - [x] Create `internal/embed/web/static/styles/input.css` with Tailwind directives
  - [x] Add build script to generate CSS
  - [x] Update Makefile with CSS build
  - [x] Fix embed directive to properly include CSS in binary

- [x] **Step 6.2: Typography & Layout Styling**
  - [x] Add Tailwind Typography plugin
  - [x] Style markdown content area
  - [x] Style navigation and tree view
  - [x] Add responsive design

- [x] **Step 6.3: Component Styling**
  - [x] Style file tree navigation
  - [x] Style breadcrumbs
  - [x] Style buttons and links
  - [x] Add hover states and transitions
  - [x] Ensure consistent spacing

---

## Phase 7: HTMX Integration

- [x] **Step 7.1: HTMX Setup**
  - [x] Gather context for htmx from `/bigskysoftware/htmx`
  - [x] Ensure htmx.js loaded in base template
  - [x] Configure htmx defaults
  - [x] Add HTMX attributes to navigation links

- [x] **Step 7.2: Partial Template Updates**
  - [x] Create partial template `MarkdownContent` for content area
  - [x] Update handlers to detect HTMX requests (check HX-Request header)
  - [x] Return full page or partial based on request
  - [x] Add proper HTMX response headers

- [x] **Step 7.3: Live Reload with SSE**
  - [x] Create SSE endpoint: `GET /events`
  - [x] Connect watcher events to SSE stream
  - [x] Add HTMX SSE extension (sse.js v2.2.2)
  - [x] Auto-refresh content on file change via SSE message trigger
  - [x] SSE connection established on body element

---

## Phase 8: Integration & Polish

- [ ] **Step 8.1: Full Integration Testing**
  - [ ] Create integration tests for full workflow
  - [ ] Test all markdown extensions together
  - [ ] Test configuration scenarios

- [ ] **Step 8.2: Error Handling**
  - [ ] Add error templates
  - [ ] Handle missing files gracefully
  - [ ] Handle invalid markdown
  - [ ] Handle configuration errors
  - [ ] Add helpful error messages

- [ ] **Step 8.3: Performance Optimization**
  - [ ] Profile with pprof
  - [ ] Optimize hot paths
  - [ ] Add caching where appropriate
  - [ ] Optimize file scanning for large directories

- [ ] **Step 8.4: Documentation**
  - [ ] Write README.md with installation and usage
  - [ ] Write CONTRIBUTING.md
  - [ ] Add code documentation where necessary for API docs
  - [ ] Create example .stigmergic.toml

---

## Phase 9: Nix Packaging

- [ ] **Step 9.1: Complete Nix Package**
  - [ ] Update `flake.nix` with complete package definition
  - [ ] Calculate correct vendorHash
  - [ ] Add metadata (description, license)
  - [ ] Test installation: `nix profile install .#`
  - [ ] Test direct run: `nix run .# -- /path/to/docs`

- [ ] **Step 9.2: CI/CD with Nix**
  - [ ] Create `.github/workflows/ci.yml` for builds and tests
  - [ ] Create `.github/workflows/release.yml` for releases

---

## Phase 10: Testing & Release

- [ ] **Step 10.1: Comprehensive Testing**
  - [ ] Run full test suite with coverage
  - [ ] Add missing tests
  - [ ] Test on Linux and macOS
  - [ ] Manual testing of all features
  - [ ] Fix any bugs found

- [ ] **Step 10.2: Beta Release**
  - [ ] Tag version v0.1.0-beta
  - [ ] Build release binaries
  - [ ] Create GitHub release
  - [ ] Share with beta testers

- [ ] **Step 10.3: v1.0.0 Release**
  - [ ] Address beta feedback
  - [ ] Final polish and testing
  - [ ] Tag v1.0.0
  - [ ] Create release announcement
  - [ ] Update documentation
