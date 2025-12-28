.PHONY: help generate build test clean css lint vendor-js

help:
	@echo "stigmergic.dev - Makefile commands"
	@echo ""
	@printf "  make build     (L%s) - Build the binary\n" "$$(grep -n '^build:' Makefile | cut -d: -f1)"
	@printf "  make test      (L%s) - Run tests\n" "$$(grep -n '^test:' Makefile | cut -d: -f1)"
	@printf "  make lint      (L%s) - Run linter\n" "$$(grep -n '^lint:' Makefile | cut -d: -f1)"
	@printf "  make generate  (L%s) - Generate templ templates\n" "$$(grep -n '^generate:' Makefile | cut -d: -f1)"
	@printf "  make css       (L%s) - Build Tailwind CSS\n" "$$(grep -n '^css:' Makefile | cut -d: -f1)"
	@printf "  make vendor-js (L%s) - Vendor JS dependencies (katex, mermaid)\n" "$$(grep -n '^vendor-js:' Makefile | cut -d: -f1)"
	@printf "  make clean     (L%s) - Clean build artifacts\n" "$$(grep -n '^clean:' Makefile | cut -d: -f1)"

lint:
	golangci-lint run --max-issues-per-linter=1 --max-same-issues=1 ./...

css:
	pnpm exec tailwindcss -i ./internal/embed/web/static/styles/input.css -o ./internal/embed/web/static/styles/output.css --minify

generate:
	templ generate

build: css generate
	go build -o stigmergic ./cmd/stigmergic

test: generate
	go test -v -race -failfast ./...

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
