.PHONY: generate build test clean css lint vendor-js

lint:
	golangci-lint run ./...

css:
	pnpm exec tailwindcss -i ./internal/embed/web/static/styles/input.css -o ./internal/embed/web/static/styles/output.css --minify

generate:
	templ generate

build: css generate
	go build -o stigmergic ./cmd/stigmergic

test: generate
	go test ./...

clean:
	rm -f stigmergic
	rm -f ./internal/embed/web/static/styles/output.css
	find . -name '*_templ.go' -delete

vendor-js:
	cp node_modules/katex/dist/katex.min.js internal/embed/web/static/js/
	cp node_modules/katex/dist/contrib/auto-render.min.js internal/embed/web/static/js/katex-auto-render.min.js
	cp node_modules/katex/dist/katex.min.css internal/embed/web/static/css/
	cp node_modules/katex/dist/fonts/*.woff2 internal/embed/web/static/fonts/
	cp node_modules/mermaid/dist/mermaid.min.js internal/embed/web/static/js/
	sed -i 's|url(fonts/|url(/static/fonts/|g' internal/embed/web/static/css/katex.min.css
