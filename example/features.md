# Features

## Live Reload

Stigmergic uses [Server-Sent Events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events) to push file changes to your browser. When you save a file, the rendered view updates immediately — no manual refresh, no polling.

The watcher uses `fsnotify` with event debouncing to avoid redundant updates. It respects `.gitignore` patterns by default, so build artifacts and `node_modules` won't trigger reloads.

## Command Palette

Press **`Ctrl+K`** (or **`⌘+K`** on Mac) to open the command palette. It searches across all markdown files in your directory by filename. Results update as you type with fuzzy matching.

This is the fastest way to navigate large documentation sets. No need to browse the file tree — just type a few characters and jump directly to the file you need.

## Markdown Rendering

Full [CommonMark](https://commonmark.org/) compliance plus GitHub Flavored Markdown extensions:

- **Tables** — pipe-delimited tables render as styled HTML
- **Task lists** — `- [x]` and `- [ ]` render as checkboxes
- **Strikethrough** — `~~deleted text~~` renders as ~~deleted text~~
- **Autolinks** — URLs automatically become clickable

## Syntax Highlighting

Code blocks with language annotations get syntax highlighting via [Chroma](https://github.com/alecthomas/chroma) with the Nord theme:

```python
def fibonacci(n):
    if n <= 1:
        return n
    return fibonacci(n - 1) + fibonacci(n - 2)
```

```go
func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintf(w, "Hello, stigmergic!")
    })
    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

Supports all major languages: Go, Python, JavaScript, TypeScript, Rust, Bash, SQL, YAML, TOML, and many more.

## Math Rendering

LaTeX equations rendered client-side with [KaTeX](https://katex.org/).

**Block equations** with `$$`:

$$
\nabla \cdot \mathbf{E} = \frac{\rho}{\varepsilon_0}
$$

**Inline math** with single `$`: The quadratic formula is $x = \frac{-b \pm \sqrt{b^2 - 4ac}}{2a}$.

## Mermaid Diagrams

[Mermaid](https://mermaid.js.org/) diagrams render directly from fenced code blocks:

```mermaid
graph LR
    A[Markdown Files] --> B[Stigmergic]
    B --> C[Beautiful Docs]
    B --> D[Live Reload]
    B --> E[Search]
```

Supports flowcharts, sequence diagrams, class diagrams, state diagrams, and more.

## Nostr Protocol Links

URLs using the `nostr:` protocol are automatically parsed and rendered as clickable links. Supports npub, note, nevent, and nprofile identifiers.

## Themes

### Built-in

- **Iceberg Dark** (default) — dark background, cool blue-grey tones
- **Iceberg Light** — light background, same color palette

### Custom Themes

Create a TOML file at `~/.config/stigmergic/themes/{name}.toml`:

```toml
name = "my-theme"

[colors]
bg = "#1e1e1e"
fg = "#d4d4d4"
alt_bg = "#252526"
blue = "#61afef"
purple = "#c678dd"

[ui]
link = "#61afef"
link_hover = "#84c5ff"
code_bg = "#2d2d2d"
border = "#3e3e3e"
```

Use with `stigmergic serve --theme my-theme ./docs`.

## Smart Defaults

- **Auto port finding** — if port 8080 is busy, finds the next available port
- **Gitignore support** — respects `.gitignore` patterns (toggle with `--respect-gitignore=false`)
- **Default file** — set `--default-file README.md` to load a specific file on startup
- **Security headers** — X-Content-Type-Options, X-Frame-Options, path traversal protection
