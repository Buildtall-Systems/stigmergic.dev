# Project Progress

**Project:** stigmergic.dev
**Type:** project

## Progress Summary

**Total Steps**: 43
**Completed**: 6
**In Progress**: 0
**Remaining**: 37

**Current Phase**: Phase 1 - CLI Foundation (Cobra + Viper)
**Current Step**: Step 1.3 - Serve Command

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

- [ ] **Step 1.3: Serve Command**
  - [ ] Create `cmd/stigmergic/serve.go` with serve command
  - [ ] Accept path as positional argument
  - [ ] Validate path exists and is a directory
  - [ ] Wire up config to serve command
  - [ ] Add basic logging of configuration

---

## Phase 2: File Tree & Watcher

- [ ] **Step 2.1: File Tree Model**
  - [ ] Create `internal/models/tree.go` with Node and Tree structs
  - [ ] Create `internal/models/tree_test.go` with comprehensive tests

- [ ] **Step 2.2: File Tree Scanner**
  - [ ] Create `internal/watcher/scanner.go` with ScanDirectory function
  - [ ] Create `internal/watcher/scanner_test.go` with comprehensive tests

- [ ] **Step 2.3: Filesystem Watcher**
  - [ ] Gather context for fsnotify from `/fsnotify/fsnotify`
  - [ ] Create `internal/watcher/watcher.go` with Watcher struct
  - [ ] Create `internal/watcher/watcher_test.go` with comprehensive tests

---

## Phase 3: HTTP Server Foundation

- [ ] **Step 3.1: Basic HTTP Server**
  - [ ] Create `internal/server/server.go` with Server struct
  - [ ] Create `internal/server/server_test.go` with comprehensive tests

- [ ] **Step 3.2: Static File Serving**
  - [ ] Create `web/static/js/` directory and add htmx
  - [ ] Create `web/static/styles/` directory
  - [ ] Add static file handler in `internal/server/handlers.go`
  - [ ] Create test to verify static files served correctly

- [ ] **Step 3.3: Middleware**
  - [ ] Create `internal/server/middleware.go` with logging, recovery, and security middleware
  - [ ] Add middleware to server
  - [ ] Test middleware behavior

---

## Phase 4: Templating with Templ

- [ ] **Step 4.1: Templ Setup**
  - [ ] Gather context for templ from `/a-h/templ`
  - [ ] Create `web/templates/base.templ` with HTML5 boilerplate
  - [ ] Add templ generation to build process
  - [ ] Create `Makefile` with generate, build, and test targets
  - [ ] Update `.gitignore` to ignore generated `*_templ.go` files

- [ ] **Step 4.2: Base Layout**
  - [ ] Create `web/templates/components/layout.templ` with base HTML structure
  - [ ] Create basic styling structure
  - [ ] Test template renders

- [ ] **Step 4.3: Home Page Template**
  - [ ] Create `web/templates/home.templ` with file tree rendering
  - [ ] Create `internal/server/handlers.go` home handler
  - [ ] Wire up route: `GET /`

---

## Phase 5: Markdown Rendering

- [ ] **Step 5.1: Basic Goldmark Setup**
  - [ ] Gather context for goldmark from `/yuin/goldmark`
  - [ ] Create `internal/markdown/parser.go` with Parse function
  - [ ] Create `internal/markdown/parser_test.go` with comprehensive tests

- [ ] **Step 5.2: Syntax Highlighting (Chroma)**
  - [ ] Gather context for chroma from `/alecthomas/chroma`
  - [ ] Update `internal/markdown/parser.go` with chroma extension
  - [ ] Create `internal/markdown/extensions.go` for extension setup
  - [ ] Test with various code languages

- [ ] **Step 5.3: GFM Extensions**
  - [ ] Add GFM extension for tables
  - [ ] Add GFM extension for strikethrough
  - [ ] Add GFM extension for task lists
  - [ ] Test each GFM feature

- [ ] **Step 5.4: Math Rendering (KaTeX)**
  - [ ] Gather context for katex from `/katex/katex`
  - [ ] Add KaTeX CSS and JS to base template
  - [ ] Create custom goldmark extension for math
  - [ ] Test math rendering

- [ ] **Step 5.5: Mermaid Diagrams**
  - [ ] Gather context for mermaid from `/mermaid-js/mermaid`
  - [ ] Add Mermaid JS to base template
  - [ ] Create custom goldmark extension for mermaid blocks
  - [ ] Test various diagram types

- [ ] **Step 5.6: Nostr URL Parsing**
  - [ ] Create custom goldmark extension for nostr URLs
  - [ ] Test nostr URL detection and linking

- [ ] **Step 5.7: Markdown View Template**
  - [ ] Create `web/templates/markdown.templ` with markdown rendering
  - [ ] Create handler for markdown files
  - [ ] Wire up route: `GET /file/*path`

---

## Phase 6: Tailwind CSS & Styling

- [ ] **Step 6.1: Tailwind Setup**
  - [ ] Gather context for tailwindcss from `/websites/tailwindcss`
  - [ ] Create `tailwind.config.js` with content paths
  - [ ] Create `web/static/styles/input.css` with Tailwind directives
  - [ ] Add build script to generate CSS
  - [ ] Update Makefile with CSS build

- [ ] **Step 6.2: Typography & Layout Styling**
  - [ ] Add Tailwind Typography plugin
  - [ ] Style markdown content area
  - [ ] Style navigation and tree view
  - [ ] Add responsive design

- [ ] **Step 6.3: Component Styling**
  - [ ] Style file tree navigation
  - [ ] Style breadcrumbs
  - [ ] Style buttons and links
  - [ ] Add hover states and transitions
  - [ ] Ensure consistent spacing

---

## Phase 7: HTMX Integration

- [ ] **Step 7.1: HTMX Setup**
  - [ ] Gather context for htmx from `/bigskysoftware/htmx`
  - [ ] Ensure htmx.js loaded in base template
  - [ ] Configure htmx defaults
  - [ ] Add HTMX attributes to navigation links

- [ ] **Step 7.2: Partial Template Updates**
  - [ ] Create `web/templates/components/content.templ` for main content area
  - [ ] Update handlers to detect HTMX requests
  - [ ] Return full page or partial based on request
  - [ ] Add proper HTMX response headers

- [ ] **Step 7.3: Live Reload with SSE**
  - [ ] Create SSE endpoint: `GET /events`
  - [ ] Connect watcher events to SSE stream
  - [ ] Add HTMX extension for SSE
  - [ ] Auto-refresh content on file change
  - [ ] Show notification on update

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
