<style>
.lp-hero { text-align: center; padding: 2.5em 1.5em 2.25em; border: 1px solid var(--border-color); border-radius: 0.75em; background: var(--bg-alt-color); margin-bottom: 2em; }
.lp-hero h1 { margin: 0 0 0.2em; font-size: 2.6em; }
.lp-tagline { font-size: 1.15em; color: var(--comment-color); margin: 0 0 0.5em; }
.lp-served { font-size: 0.85em; color: var(--comment-color); margin: 0 0 1.6em; }
.lp-cta { display: inline-block; margin: 0 0.35em; padding: 0.55em 1.3em; border-radius: 0.5em; font-weight: 600; text-decoration: none; border: 1px solid var(--border-color); }
.lp-cta-primary { background: var(--green-color); color: var(--bg-color); }
a.lp-cta-primary:hover { color: var(--bg-color); opacity: 0.85; }
.lp-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(17em, 1fr)); gap: 0.9em; margin: 1.5em 0; }
.lp-card { border: 1px solid var(--border-color); border-radius: 0.5em; padding: 0.9em 1em; background: var(--bg-alt-color); }
.lp-card h3 { margin: 0 0 0.35em; font-size: 1em; }
.lp-card p { margin: 0; font-size: 0.88em; color: var(--comment-color); line-height: 1.45; }
.lp-keys { display: flex; flex-direction: column; gap: 0.55em; width: fit-content; margin: 1.4em 0; padding: 0; list-style: none; }
.lp-keys li { display: grid; grid-template-columns: 7em 1fr; gap: 0.9em; align-items: baseline; font-size: 0.88em; color: var(--comment-color); }
.lp-keys .lp-key { text-align: right; }
.lp-keys kbd { padding: 0.12em 0.5em; border: 1px solid var(--border-color); border-radius: 0.3em; background: var(--bg-alt-color); font-size: 0.85em; color: var(--fg-color); font-family: monospace; }
</style>

<div class="lp-hero">
  <h1>Stigmergic</h1>
  <p class="lp-tagline">A fast, beautiful markdown viewer with live reload.</p>
  <p class="lp-served">You are looking at it — this site is markdown served by Stigmergic itself.</p>
  <p>
    <a class="lp-cta lp-cta-primary" href="https://github.com/Buildtall-Systems/stigmergic.dev/releases/latest">Download</a>
    <a class="lp-cta" href="https://github.com/Buildtall-Systems/stigmergic.dev">GitHub</a>
  </p>
</div>

Stigmergic watches a directory of markdown files and serves them through a local web server. Edit a file and the page updates over [Server-Sent Events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events) — no refresh, no build step. Point it at project docs, research notes, AI-generated documentation, or a personal wiki, and you have a live documentation site in seconds.

## What You Get

<div class="lp-grid">
  <div class="lp-card">
    <h3>Live Reload</h3>
    <p>Edit a file, watch the page update instantly. Server-Sent Events push every change; the view refreshes only when the document you are reading changed.</p>
  </div>
  <div class="lp-card">
    <h3>Command Palette</h3>
    <p>Ctrl+K opens fuzzy search across every file in the corpus. Type a few characters, hit Enter, you are there.</p>
  </div>
  <div class="lp-card">
    <h3>Reader Mode</h3>
    <p>One keystroke collapses both navigation rails and scales the document up — full-width, distraction-free reading with the header intact.</p>
  </div>
  <div class="lp-card">
    <h3>Keyboard Reading</h3>
    <p>Leap between sections with n and p. The outline rail tracks your position as you scroll and jumps on click.</p>
  </div>
  <div class="lp-card">
    <h3>Follow Mode</h3>
    <p>The view follows whatever file changed most recently — watch an agent write documentation in real time.</p>
  </div>
  <div class="lp-card">
    <h3>Math &amp; Diagrams</h3>
    <p>LaTeX equations via KaTeX, flowcharts and sequence diagrams via Mermaid, wireframes via wiremd. Write fences, see renderings.</p>
  </div>
  <div class="lp-card">
    <h3>Wiki Links &amp; Backlinks</h3>
    <p>[[target]] links resolve across the corpus, and every document lists the pages that link to it.</p>
  </div>
  <div class="lp-card">
    <h3>Syntax Highlighting</h3>
    <p>Code blocks in any language, highlighted server-side with theme-matched colors. Toggle any page to its raw markdown source.</p>
  </div>
  <div class="lp-card">
    <h3>Themes</h3>
    <p>Iceberg Dark and Light ship in the binary; define your own palette in a TOML file. Cycling is one keypress.</p>
  </div>
  <div class="lp-card">
    <h3>Nostr Links</h3>
    <p>Native rendering of nostr: protocol URIs — npubs, notes, and events become clickable links.</p>
  </div>
</div>

The whole interface is reachable from the keyboard:

<ul class="lp-keys">
  <li><span class="lp-key"><kbd>Ctrl+K</kbd></span><span>search</span></li>
  <li><span class="lp-key"><kbd>N</kbd> / <kbd>P</kbd></span><span>sections</span></li>
  <li><span class="lp-key"><kbd>R</kbd></span><span>reader mode</span></li>
  <li><span class="lp-key"><kbd>F</kbd></span><span>follow mode</span></li>
  <li><span class="lp-key"><kbd>S</kbd></span><span>source view</span></li>
  <li><span class="lp-key"><kbd>T</kbd></span><span>theme</span></li>
  <li><span class="lp-key"><kbd>?</kbd></span><span>all shortcuts</span></li>
</ul>

## Quick Start

```bash
# Nix
nix profile install github:Buildtall-Systems/stigmergic.dev

# Go
go install github.com/Buildtall-Systems/stigmergic.dev/cmd/stigmergic@latest
```

Pre-built binaries for Linux, macOS, and Windows are on the [releases page](https://github.com/Buildtall-Systems/stigmergic.dev/releases/latest), checksums included — see [[installation]] for all install options and configuration.

```bash
stigmergic serve /path/to/your/markdown
```

The server starts at `http://localhost:8080` and picks the next free port automatically if that one is taken.

## Explore This Site

- [[features]] — complete feature overview with examples
- [[installation]] — install options and setup details
- [[architecture]] — how Stigmergic is structured internally
- [[demo]] — every supported rendering feature on one page

## Built With

[Go](https://go.dev/), [HTMX](https://htmx.org), [Templ](https://templ.guide), [Goldmark](https://github.com/yuin/goldmark), [Tailwind CSS](https://tailwindcss.com), [KaTeX](https://katex.org), [Mermaid](https://mermaid.js.org). No JavaScript frameworks, no database, no build pipeline for your content — markdown files and a single Go binary, version-controlled with git if you want it to be.

---

*Stigmergic is named after the biological phenomenon where organisms coordinate through environmental traces — ants leaving pheromones, termites building mounds. Your markdown files are the traces. Stigmergic makes them visible.*

[GitHub Repository](https://github.com/Buildtall-Systems/stigmergic.dev) · [Releases](https://github.com/Buildtall-Systems/stigmergic.dev/releases) · [Issues](https://github.com/Buildtall-Systems/stigmergic.dev/issues) · [Buildtall Systems](https://buildtall.systems)
