.PHONY: generate build test clean

generate:
	templ generate

build: generate
	go build -o stigmergic ./cmd/stigmergic

test: generate
	go test ./...

clean:
	rm -f stigmergic
	find . -name '*_templ.go' -delete
