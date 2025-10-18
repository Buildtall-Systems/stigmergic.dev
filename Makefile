.PHONY: generate build test clean css

css:
	tailwindcss -i ./web/static/styles/input.css -o ./web/static/styles/output.css --minify

generate:
	templ generate

build: css generate
	go build -o stigmergic ./cmd/stigmergic

test: generate
	go test ./...

clean:
	rm -f stigmergic
	rm -f ./web/static/styles/output.css
	find . -name '*_templ.go' -delete
