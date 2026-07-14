BINARY  := terraform-provider-ripe-atlas
MODULE  := github.com/supabase/terraform-provider-ripe-atlas

GOFLAGS ?=
VERSION ?= $(shell git describe --tags --match "v*" 2>/dev/null || echo "v0.0.0-dev")
LDFLAGS := -s -w

OS_ARCH := $(shell go env GOOS)_$(shell go env GOARCH)
INSTALL_DIR := ~/.terraform.d/plugins/registry.terraform.io/supabase/ripe-atlas/$(VERSION:v%=%)/$(OS_ARCH)

.PHONY: build test vet lint install clean

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)

clean:
	rm -f $(BINARY)
