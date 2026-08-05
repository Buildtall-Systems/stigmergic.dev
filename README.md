# Stigmergic

A dynamic markdown file watcher and renderer for developers working with voluminous markdown documentation, particularly in agentic coding workflows.

Stigmergic watches a directory tree for markdown files and serves them beautifully through a local HTTP server with real-time updates. Perfect for viewing AI-generated documentation, project notes, research collections, or any other markdown-heavy workflows.

## Features

- **Live Reload**: Automatically detects file changes and updates the browser in real-time
- **Directory Browsing**: Intuitive file tree navigation with expandable folders
- **Syntax Highlighting**: Code blocks highlighted using Chroma with the Nord theme
- **Math Rendering**: LaTeX equations rendered beautifully with KaTeX
- **Diagram Support**: Mermaid diagrams for flowcharts, sequence diagrams, and more
- **Nostr Protocol Links**: Native support for Nostr protocol link rendering
- **Transclusion**: `![[note]]`, `![[note#section]]`, and `![[image.png]]` wiki-link embeds render notes, sections, images, and attachments inline
- **Fast Navigation**: HTMX-powered partial page updates for smooth browsing
- **Theme System**: Built-in themes (Iceberg Dark/Light) with custom theme support
- **Smart Filtering**: Respects `.gitignore` patterns by default
- **Auto Port Finding**: Automatically finds an available port if the default is busy

## Installation

### From Source

```bash
git clone https://github.com/Buildtall-Systems/stigmergic.dev.git
cd stigmergic.dev
go build -o stigmergic ./cmd/stigmergic
```

### Using Nix Flake

```bash
nix profile install github:Buildtall-Systems/stigmergic.dev
```

Or add to your `flake.nix`:

```nix
{
  inputs = {
    stigmergic.url = "github:Buildtall-Systems/stigmergic.dev";
  };
}
```

### Using Go Install

```bash
go install github.com/Buildtall-Systems/stigmergic.dev/cmd/stigmergic@latest
```

## Usage

### Basic Usage

```bash
stigmergic serve /path/to/markdown/docs
```

This starts a local server at `http://localhost:8080` watching the specified directory. Running bare `stigmergic` is equivalent to `stigmergic serve .`.

### Content Sources

One binary, two commands, one rendering pipeline:

- **`stigmergic serve [path]`** renders a live directory: an fsnotify watcher pushes reloads over SSE, `.gitignore` filtering can be toggled at runtime, recently updated files are surfaced, and a copy-path button is available.
- **`stigmergic site`** renders the public stigmergic.dev website from content compiled into the binary (`site/content/`, embedded via `go:embed`). No filesystem access, no watcher goroutines; UI features tied to a live filesystem are absent. Auth and all other config apply identically in both modes.

The server core is source-agnostic — content providers implement the `ContentSource` interface in `internal/source`, advertising optional capabilities (watchability, gitignore awareness, meaningful mod times, a local root) that gate the corresponding UI features.

### Command-Line Options

```bash
stigmergic serve [path] [flags]
```

**Flags:**

- `-p, --port` - Server port (default: 8080)
- `--host` - Server host (default: localhost)
- `-c, --config` - Configuration file path
- `--log-level` - Logging level: DEBUG, INFO, WARN, ERROR (default: ERROR)
- `--respect-gitignore` - Use .gitignore patterns (default: true)

**Examples:**

```bash
stigmergic serve ./docs

stigmergic serve --port 3000 --host 0.0.0.0 ./notes

stigmergic serve --config ~/.stigmergic.toml ~/research

stigmergic serve --log-level INFO --respect-gitignore=false ./all-files
```

### Configuration File

Stigmergic supports configuration via TOML file at:
- `./.stigmergic.toml` (current directory)
- `~/.config/stigmergic/config.toml` (user config)
- Path specified with `--config` flag

**Example configuration:**

```toml
port = 8080
host = "localhost"
loglevel = "ERROR"
respectgitignore = true
theme = "iceberg-dark"
defaultfile = "index.md"
base_url = ""

ignorepatterns = [
    ".git",
    "node_modules",
    "*.tmp"
]
```

### Environment Variables

Configuration can also be set via environment variables with the `STIGMERGIC_` prefix:

```bash
export STIGMERGIC_PORT=3000
export STIGMERGIC_LOGLEVEL=DEBUG
stigmergic serve ./docs
```

## Themes

### Built-in Themes

- `iceberg-dark` (default)
- `iceberg-light`

### Custom Themes

Create custom themes at `~/.config/stigmergic/themes/{name}.toml`:

```toml
name = "my-theme"

[colors]
bg = "#1e1e1e"
fg = "#d4d4d4"
alt_bg = "#252526"
comment = "#6a9955"
red = "#f48771"
orange = "#ff9800"
yellow = "#e5c07b"
green = "#98c379"
cyan = "#56b6c2"
blue = "#61afef"
purple = "#c678dd"

[ui]
link = "#61afef"
link_hover = "#84c5ff"
code_bg = "#2d2d2d"
border = "#3e3e3e"
selection_bg = "#264f78"
```

Use with `--theme my-theme` or set in config file.

## Development

### Prerequisites

- Go 1.24+
- Node.js (for Tailwind CSS)

### Using Nix

The easiest way to get a consistent development environment:

```bash
nix develop
```

Or with direnv:

```bash
echo "use flake" > .envrc
direnv allow
```

### Manual Setup

Install dependencies:

```bash
go mod download
go install github.com/a-h/templ/cmd/templ@latest
npm install -g tailwindcss
```

### Build Process

```bash
templ generate

tailwindcss -i ./internal/embed/web/static/styles/input.css \
            -o ./internal/embed/web/static/styles/output.css \
            --minify

go build -o stigmergic ./cmd/stigmergic
```

### Running Tests

```bash
go test ./...

go test -race ./...
```

### Live Development

Watch templates and rebuild:

```bash
templ generate --watch
```

In another terminal:

```bash
go run ./cmd/stigmergic serve ./docs
```

## How It Works

1. **File Watching**: Uses fsnotify to monitor the directory tree for changes
2. **Markdown Parsing**: Goldmark parses markdown with GFM, syntax highlighting, and extensions
3. **HTML Rendering**: Templ generates type-safe HTML templates
4. **Live Updates**: Server-Sent Events (SSE) push file change notifications to connected clients
5. **Progressive Enhancement**: HTMX handles navigation and content updates without full page refreshes

## Use Cases

- **Agentic Coding Workflows**: View markdown documentation generated by AI coding assistants
- **Research Notes**: Browse and organize research collections with live updates
- **Project Documentation**: Real-time preview of documentation as you write
- **Knowledge Bases**: Navigate large collections of interconnected markdown files
- **Meeting Notes**: View and organize meeting notes with automatic refresh
- **Personal Wiki**: Lightweight personal wiki with file-based storage

## Architecture

- **CLI**: Cobra framework for command parsing
- **Config**: Viper for hierarchical configuration (flags > env > file > defaults)
- **Watcher**: fsnotify with event debouncing and gitignore support
- **Server**: Standard library HTTP server with middleware stack
- **Markdown**: Goldmark with Chroma highlighting, math (KaTeX), and Mermaid extensions
- **Templates**: a-h/templ for type-safe HTML generation
- **Frontend**: HTMX for dynamic content, Tailwind CSS for styling

## Security

- Path traversal protection prevents accessing files outside the watched directory
- Security headers (X-Content-Type-Options, X-Frame-Options, X-XSS-Protection)
- Recovery middleware catches panics and returns safe error pages
- Read-only access to filesystem (no file writing or modification)

## Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.

## License

MIT

## Credits

Built with:
- [Goldmark](https://github.com/yuin/goldmark) - Markdown parser
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Viper](https://github.com/spf13/viper) - Configuration management
- [fsnotify](https://github.com/fsnotify/fsnotify) - File system notifications
- [templ](https://github.com/a-h/templ) - Go templating language
- [HTMX](https://htmx.org) - HTML over the wire
- [Tailwind CSS](https://tailwindcss.com) - Utility-first CSS
- [Chroma](https://github.com/alecthomas/chroma) - Syntax highlighting
- [KaTeX](https://katex.org) - Math rendering
- [Mermaid](https://mermaid.js.org) - Diagram rendering

## Support

For issues, questions, or feature requests, please open an issue on GitHub.

---

**Stigmergic** - Named after the biological phenomenon where organisms coordinate through environmental traces, this tool helps your markdown documents emerge into a coherent, navigable knowledge base.
