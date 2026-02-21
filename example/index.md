# stigmergic

A dynamic markdown watcher and renderer for developers working with voluminous markdown documentation.

Stigmergic watches a directory tree for markdown files and serves them through a local HTTP server with real-time updates. Built for agentic coding workflows, research collections, and documentation-heavy projects.

## Download

Pre-built binaries are available for every release on GitHub.

### Quick Install

Download the latest release for your platform:

| Platform | Architecture | Download |
|----------|-------------|----------|
| Linux | x86_64 | [stigmergic_linux_amd64.tar.gz](https://github.com/Buildtall-Systems/stigmergic.dev/releases/latest/download/stigmergic_linux_amd64.tar.gz) |
| Linux | ARM64 | [stigmergic_linux_arm64.tar.gz](https://github.com/Buildtall-Systems/stigmergic.dev/releases/latest/download/stigmergic_linux_arm64.tar.gz) |
| macOS | Apple Silicon | [stigmergic_darwin_arm64.tar.gz](https://github.com/Buildtall-Systems/stigmergic.dev/releases/latest/download/stigmergic_darwin_arm64.tar.gz) |
| macOS | Intel | [stigmergic_darwin_amd64.tar.gz](https://github.com/Buildtall-Systems/stigmergic.dev/releases/latest/download/stigmergic_darwin_amd64.tar.gz) |
| Windows | x86_64 | [stigmergic_windows_amd64.zip](https://github.com/Buildtall-Systems/stigmergic.dev/releases/latest/download/stigmergic_windows_amd64.zip) |

### Manual Install

1. Download the archive for your platform from the table above
2. Extract it:

```bash
# Linux / macOS
tar xzf stigmergic_*.tar.gz

# Windows (PowerShell)
Expand-Archive stigmergic_*.zip -DestinationPath .
```

3. Move the binary to a directory on your `PATH`:

```bash
# Linux / macOS
sudo mv stigmergic /usr/local/bin/

# Or, without root:
mv stigmergic ~/.local/bin/
```

4. Verify the installation:

```bash
stigmergic --version
```

### Verify Checksums

Each release includes a `checksums.txt` file signed with SHA-256. To verify your download:

```bash
curl -sL https://github.com/Buildtall-Systems/stigmergic.dev/releases/latest/download/checksums.txt -o checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

### Alternative Install Methods

**Nix Flake** (declarative, reproducible):

```bash
nix profile install github:Buildtall-Systems/stigmergic.dev
```

**Go Install** (requires Go toolchain):

```bash
go install github.com/Buildtall-Systems/stigmergic.dev/cmd/stigmergic@latest
```

## Usage

Point stigmergic at any directory containing markdown files:

```bash
stigmergic serve ./docs
```

Then open `http://localhost:8080` in your browser. Files are watched in real-time — edits appear instantly.

### Options

```
-p, --port           Server port (default: 8080)
    --host           Bind address (default: localhost)
-c, --config         Configuration file path
    --log-level      DEBUG, INFO, WARN, ERROR (default: ERROR)
    --respect-gitignore  Use .gitignore patterns (default: true)
    --default-file   File to display on homepage
```

## Features

- **Live Reload** via Server-Sent Events
- **Syntax Highlighting** with Chroma (Nord theme)
- **Math Rendering** with KaTeX
- **Mermaid Diagrams** for flowcharts and sequence diagrams
- **Nostr Protocol Links** rendered natively
- **Command Palette** for fast file navigation
- **Theme System** with custom theme support
- **.gitignore Aware** filtering by default

## Source

[github.com/Buildtall-Systems/stigmergic.dev](https://github.com/Buildtall-Systems/stigmergic.dev)
