VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    := $(shell date -u +%Y-%m-%dT%H:%MZ)
LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.buildDate=$(DATE)

# Build local (dev).
build:
	go build -ldflags="$(LDFLAGS)" -o deployeur .

# Binaire statique pour les serveurs (Linux amd64), aucune dépendance runtime.
release:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o deployeur .

vet:
	gofmt -l . && go vet ./...

clean:
	rm -f deployeur

.PHONY: build release vet clean
