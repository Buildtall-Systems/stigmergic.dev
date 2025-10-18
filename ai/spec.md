# stigmergic.dev - Project Specification

**Repository**: git@github.com:Buildtall-Systems/stigmergic.dev.git

## Overview

stigmergic.dev is a command-line tool for watching and dynamically rendering markdown files from the filesystem through a local web interface. It provides a beautiful, real-time markdown reading experience with automatic updates as files change.

## Core Concept

When run on a path, the tool:
1. Spins up a lightweight local HTTP server (Go)
2. Watches the filesystem for changes
3. Presents a tree-structured navigation of markdown files
4. Renders markdown beautifully when clicked
5. Auto-updates the view when files change

## Technology Stack

- **Backend**: Go HTTP server
- **CLI Framework**: spf13/cobra
- **Configuration**: spf13/viper
- **Templating**: templ (templ.guide)
- **Styling**: Tailwind CSS
- **Interactivity**: HTMX
- **Markdown**: goldmark parser
- **Package Management**: Nix (flake.nix for dev env and installation)

## Features

### File Watching & Navigation

- Real-time filesystem watching with automatic UI updates
- Tree-structured navigation mirroring directory structure on home page
- Directory navigation shows listing of files and subdirectories
- Clicking markdown files renders them in-place
- No ignore patterns in initial version

### Markdown Rendering

The markdown renderer must support:

- **Standard Markdown**: Full CommonMark compliance
- **Syntax Highlighting**: Language-specific code block highlighting
- **Mermaid Diagrams**: Render Mermaid diagram syntax as visual diagrams
- **Math/LaTeX**: Mathematical equations using LaTeX syntax
- **Tables & Task Lists**: GitHub Flavored Markdown tables and task lists
- **Nostr URLs**: Parse and link nostr:// URLs (e.g., nostr:npub, nostr:note, nostr:nevent)

### Configuration

Support both configuration file and CLI flags using **cobra** (CLI) and **viper** (config management). CLI flags take precedence over config file values.

#### Configuration File (via viper)
- Optional `.stigmergic.toml` in watched directory or `~/.config/stigmergic/config.toml`
- TOML format for human-friendly configuration
- Configurable options:
  - Port number
  - Host binding
  - Theme/styling preferences
  - Markdown rendering options

#### CLI Flags (via cobra)
```
stigmergic [path] [flags]

Flags:
  --port, -p      Server port (default: 8080)
  --host, -h      Host to bind to (default: localhost)
  --config, -c    Path to config file (default: .stigmergic.toml)
```

#### Configuration Precedence (viper standard)
1. CLI flags (highest priority)
2. Environment variables (prefixed with STIGMERGIC_)
3. Config file
4. Default values (lowest priority)

### User Interface

#### Home Page
- Tree view of all markdown files in watched directory
- Expandable/collapsible directory structure
- Visual indicators for directories vs files
- Clean, minimal design
- Fast, responsive navigation

#### Markdown View
- Beautiful, readable typography
- Proper heading hierarchy
- Syntax-highlighted code blocks with copy button
- Rendered Mermaid diagrams
- Rendered LaTeX equations
- Styled tables and task lists
- Clickable nostr URLs
- Breadcrumb navigation
- Back to home link

#### Real-time Updates
- HTMX-powered live updates when files change
- No full page refreshes
- Smooth transitions
- Visual feedback for updates

## Technical Architecture

### Packaging & Distribution

**Static Asset Embedding**: All static files (htmx.min.js, CSS) are embedded into the binary using Go's `embed` package. This ensures the binary is self-contained and can run from any directory without requiring external file dependencies.

### Server Components

```
stigmergic/
├── cmd/
│   └── stigmergic/
│       └── main.go           # CLI entry point
├── internal/
│   ├── server/
│   │   ├── server.go         # HTTP server setup
│   │   ├── handlers.go       # Route handlers
│   │   └── middleware.go     # Middleware (logging, etc.)
│   ├── watcher/
│   │   └── watcher.go        # Filesystem watcher
│   ├── markdown/
│   │   ├── parser.go         # goldmark parser setup
│   │   ├── extensions.go     # Custom extensions (nostr, etc.)
│   │   └── renderer.go       # HTML rendering
│   ├── config/
│   │   └── config.go         # Configuration loading
│   └── models/
│       └── tree.go           # File tree structures
├── web/
│   ├── templates/            # templ templates
│   │   ├── base.templ
│   │   ├── home.templ
│   │   ├── markdown.templ
│   │   └── components/
│   ├── static/               # Embedded into binary via go:embed
│   │   ├── styles/
│   │   │   └── tailwind.css
│   │   └── js/
│   │       └── htmx.min.js
│   └── assets/               # Compiled assets
├── flake.nix                 # Nix flake for dev env
└── README.md
```

### Data Flow

1. **Startup**:
   - Parse CLI flags and config file
   - Initialize filesystem watcher on target path
   - Build initial file tree
   - Start HTTP server

2. **Navigation Request**:
   - User clicks link in tree
   - HTMX sends GET request
   - Server determines if directory or markdown file
   - Returns appropriate templ template
   - HTMX swaps content in-place

3. **File Change**:
   - Filesystem watcher detects change
   - Server broadcasts update via SSE or WebSocket
   - HTMX polls or receives push event
   - Affected portions of UI update

### Markdown Processing Pipeline

```
.md file → goldmark parser → AST → extensions → HTML → templ → rendered output
```

Extensions to implement:
- Syntax highlighting (chroma or similar)
- Mermaid diagram detection and rendering
- LaTeX math rendering (KaTeX or MathJax)
- Nostr URL detection and linking
- GFM tables and task lists

## Nix Integration

### Development Environment (flake.nix)

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            gotools
            templ
            tailwindcss
            nodejs  # for tailwind build
          ];
        };

        packages.default = pkgs.buildGoModule {
          pname = "stigmergic";
          version = "0.1.0";
          src = ./.;
          vendorHash = null;  # Update after go mod vendor
        };
      }
    );
}
```

### Installation

Users should be able to:
1. Clone repo: `git clone git@github.com:Buildtall-Systems/stigmergic.dev.git`
2. Install via Nix: `nix profile install .#`
3. Run: `stigmergic /path/to/docs`

Or directly: `nix run github:Buildtall-Systems/stigmergic.dev -- /path/to/docs`

## Design Principles

1. **Simple**: Minimal configuration, works out of the box
2. **Fast**: Instant startup, responsive navigation, efficient file watching
3. **Beautiful**: Clean typography, proper spacing, professional appearance
4. **Reliable**: Robust file watching, graceful error handling
5. **Extensible**: Easy to add new markdown extensions or features

## Performance Targets

- Server startup: < 100ms
- Initial file tree generation: < 500ms for 1000 files
- Markdown rendering: < 50ms for typical file
- File change detection to UI update: < 200ms
- Memory footprint: < 50MB for typical use

## Security Considerations

- Only serve files within specified directory (no path traversal)
- All internal paths stored as relative paths from watch root (never absolute)
- Path validation ensures requests stay within watch directory
- Bind to localhost by default (no external exposure)
- Sanitize markdown output (XSS prevention)
- Rate limiting on file watching to prevent DOS
- No arbitrary code execution in markdown

## Future Enhancements (Not in v1)

- Full-text search across markdown files
- Dark mode toggle
- Export to PDF
- Mobile-responsive design
- Multiple directory watching
- Bookmark/favorites system
- Custom CSS injection
- Plugin system for markdown extensions
- Collaborative viewing (multi-user)
- File ignore patterns (.gitignore, .stigmergicignore)

## Success Criteria

The project is successful when:
1. User can run `stigmergic /path/to/docs` and immediately get a working server
2. All markdown files in directory are accessible and beautifully rendered
3. File changes appear in browser within 1 second
4. All specified markdown extensions work correctly
5. Installation via Nix is smooth and reliable
6. Zero-config usage works for 90% of use cases

## Testing Strategy

### Test Requirements

All code must have comprehensive test coverage with proper test isolation:

#### Unit Tests
- Markdown parsing and extensions
- Configuration loading (viper)
- CLI argument parsing (cobra)
- File tree generation
- HTTP handlers
- Each test must be isolated and not depend on other tests
- Use table-driven tests for multiple scenarios
- Mock external dependencies (filesystem, network)

#### Integration Tests
- File watching with real filesystem operations
- Full HTTP server lifecycle
- Configuration precedence (flags → env → config → defaults)
- Each integration test must use isolated temporary directories
- Clean up all resources after each test (defer cleanup)
- No shared state between tests

#### End-to-End Tests
- Full user workflows from CLI to rendered output
- Browser automation for UI interactions
- Real markdown rendering with all extensions
- Use isolated test directories with known fixture files
- Parallel test execution must be safe

#### Test Isolation Requirements
- **No global state**: Tests must not share global variables
- **Temporary directories**: Each test creates its own temp dir via `t.TempDir()`
- **Random ports**: HTTP server tests use port 0 or find available ports
- **Cleanup**: All resources cleaned up with `t.Cleanup()` or `defer`
- **Parallel safe**: Tests should be safe to run with `t.Parallel()`
- **No test order dependencies**: Tests must pass in any order
- **Idempotent**: Running tests multiple times produces same results

#### Performance Benchmarks
- Benchmark large directories (1000+ files)
- Markdown rendering performance
- File watcher throughput
- Memory profiling for leak detection

#### Additional Testing
- Browser testing for UI components (HTMX interactions)
- Nix build verification in CI
- Cross-platform testing (Linux, macOS, potentially Windows)

## Milestones

1. **M1 - Foundation**: Basic Go server, templ setup, simple markdown rendering
2. **M2 - Navigation**: File tree, HTMX integration, directory navigation
3. **M3 - Extensions**: All markdown extensions working (syntax, mermaid, latex, nostr)
4. **M4 - Watching**: Real-time file watching and UI updates
5. **M5 - Configuration**: Config file and CLI flag support
6. **M6 - Polish**: Styling, error handling, edge cases
7. **M7 - Nix**: Complete Nix integration for dev and installation
8. **M8 - Release**: Documentation, testing, v1.0.0 release
