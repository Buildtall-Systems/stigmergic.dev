# Stigmergic Demo Corpus

Sample content for trying Stigmergic against a live directory:

```bash
stigmergic serve ./example
```

- [[demo]] — every supported rendering feature on one page: GFM tables and task lists, syntax highlighting, KaTeX math, Mermaid diagrams, wiremd wireframes, Nostr links
- `.stigmergic.toml` — a minimal config showing `defaultfile`

Images render inline:

![Stigmergic rendering markdown with the Iceberg Dark theme](img/stigmergic.png)

Edit any file here while the server runs to see live reload. The public stigmergic.dev website is separate content, compiled into the binary and served by `stigmergic site`.
