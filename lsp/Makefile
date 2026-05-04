.PHONY: build test clean install

PREFIX ?= $(HOME)/.local

build:
	go build -o bin/mdx ./cmd/mdx

test:
	go test ./...

clean:
	rm -rf bin

install: build
	install -d $(PREFIX)/bin
	install -m 0755 bin/mdx $(PREFIX)/bin/mdx
