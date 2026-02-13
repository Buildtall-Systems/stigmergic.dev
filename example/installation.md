# Installation

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

Requires Go 1.24+:

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

- Go 1.24+
- Node.js (for Tailwind CSS)
- [templ](https://templ.guide) — `go install github.com/a-h/templ/cmd/templ@latest`

### Development Environment

Using Nix:

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
stigmergic serve [path] [flags]

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

Put nginx or caddy in front for TLS termination.
