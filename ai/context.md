# Context

context is king. we need it because AI coding agents have dated memories that are incomplete and not up to date, and they are unaware of this. so every time we use an API or language or anything, we need to ensure we're working with the right interface.

to that end we have a process.

## The Process

upon looking for any context, the process is to reference this file ( ./ai/context.md ). you will look for the library name or key word in the first column. if you find it, proceed through the columns to the right of it untill you see a useful entry. if there are none, you need to halt and ask the operator for help looking up the concept. they will direct you from there.

**CRITICAL: You are NEVER permitted to use web searches, WebFetch, or any web-based tools for gathering context. Context gathering MUST ALWAYS use either Context7 or a fallback local directory reference.**

if you find an entry in the "Context7 ID" column, you may immediately call the `context7` mcp tool "get-library-docs" using the entry you found in that column, and it will return the documents. if you find an entry in the "Local Reference Path" column, you may take that as a relative path from the project root to documentation files on the filesystem. you may use your file reading, grepping, listing, etc tools on this path. it will have a symlink to the source code

if you do not find an entry that matches your search term in the left-most column of the table, you should use the `context7` mcp tool  "resolve-library-id" to see if an entry exists. whether it exists or not, you should add a row to the table with that search term in the leftmost column. if it exists in context7, add the library-id to the second column. if it did not exist in context7, add that row and halt after informing the operator of the situation.

empty fields should be left empty... no "NA" or "*Not Found*"... nothing, just a space and move on.

## the index

| Library Name Searched | Context7 ID                    | Local Reference Path               |
|-----------------------|--------------------------------|------------------------------------|
| templ                 | /a-h/templ                     |                                    |
| htmx                  | /bigskysoftware/htmx           |                                    |
| goldmark              | /yuin/goldmark                 |                                    |
| cobra                 | /spf13/cobra                   |                                    |
| viper                 | /spf13/viper                   |                                    |
| chroma                | /alecthomas/chroma             |                                    |
| mermaid               | /mermaid-js/mermaid            |                                    |
| katex                 | /katex/katex                   |                                    |
| fsnotify              | /fsnotify/fsnotify             |                                    |
| tailwindcss           | /websites/tailwindcss          |                                    |
| nixpkgs               | /nixos/nixpkgs                 |                                    |
| go embed              |                                | ai/context/embed                   |
| nostr                 | /nostr-protocol/nips           |                                    |
| iceberg.vim           |                                | ai/context/iceberg.vim             |
| alpine.js             | /alpinejs/alpine               |                                    |
| go stdlib             | /golang/go                     |                                    |
| wiremd                |                                | ai/context/wiremd/                 |
| goldmark wikilink     |                                | vendor/go.abhg.dev/goldmark/wikilink/ |
| goldmark frontmatter  |                                | vendor/go.abhg.dev/goldmark/frontmatter/ |
