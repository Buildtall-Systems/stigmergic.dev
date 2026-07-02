# Installation

## Pre-built Binaries

Every [release](https://github.com/Buildtall-Systems/stigmergic.dev/releases) ships binaries for Linux (x86_64, ARM64), macOS (Apple Silicon, Intel), and Windows (x86_64), with a `checksums.txt` for verification:

```bash
tar xzf stigmergic_*.tar.gz
sudo mv stigmergic /usr/local/bin/
```

## Nix (Recommended)

The simplest way to install stigmergic. Works on Linux and macOS.

```bash
nix profile install github:Buildtall-Systems/stigmergic.dev
```

Or run without installing:

```bash
nix run github:Buildtall-Systems/stigmergic.dev -- serve ./docs
```

### Nix Flake Integration

Add to your project's `flake.nix`:

```nix
{
  inputs = {
    stigmergic.url = "github:Buildtall-Systems/stigmergic.dev";
  };

  outputs = { self, nixpkgs, stigmergic, ... }: {
    # Use stigmergic.packages.${system}.default
  };
}
```

Or use with `nix develop` for a dev environment that includes stigmergic.

## Go Install

Requires [Go](https://go.dev/) 1.24+:

```bash
go install github.com/Buildtall-Systems/stigmergic.dev/cmd/stigmergic@latest
```

Binary installs to `$GOPATH/bin/stigmergic`.

## From Source

```bash
git clone https://github.com/Buildtall-Systems/stigmergic.dev
cd stigmergic.dev
make build
make install  # installs to ~/.local/bin
```

### Build Dependencies

- [Go](https://go.dev/) 1.24+
- [Node.js](https://nodejs.org/) (for [Tailwind CSS](https://tailwindcss.com/))
- [templ](https://templ.guide) — `go install github.com/a-h/templ/cmd/templ@latest`

### Development Environment

Using [Nix](https://nixos.org/):

```bash
nix develop
```

This provides all build dependencies automatically. Then:

```bash
make build    # Build binary
make test     # Run tests
make lint     # Run linter
```

## Verify Installation

```bash
stigmergic version
stigmergic serve --help
```

## CLI Reference

```
stigmergic [path]             Same as "stigmergic serve [path]"
stigmergic serve [path]       Watch and render a directory of markdown
stigmergic site               Serve the built-in stigmergic.dev website

Arguments:
  path                        Directory to watch (default: current directory)

Flags:
  -p, --port int              Server port (default: 8080)
      --host string           Bind address (default: "localhost")
  -c, --config string         Config file path
      --default-file string   File to display on homepage
      --log-level string      DEBUG, INFO, WARN, ERROR (default: "ERROR")
      --respect-gitignore     Use .gitignore patterns (default: true)
```

`serve` renders a live directory: file changes reload the browser, `.gitignore` filtering can be toggled at runtime, and recently updated files are surfaced. `site` renders content compiled into the binary — the pages of this website — so those live-filesystem features do not apply.

## Running as a Service

For production hosting (e.g., serving a documentation site):

```ini
# /etc/systemd/system/stigmergic.service
[Unit]
Description=Stigmergic Markdown Server
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/stigmergic serve --host 0.0.0.0 --port 8080 --default-file index.md /path/to/content
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now stigmergic
```

Put [nginx](https://nginx.org/) or [Caddy](https://caddyserver.com/) in front for TLS termination.
