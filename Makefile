VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

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
