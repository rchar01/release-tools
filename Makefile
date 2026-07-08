SHELL := /bin/sh
.DEFAULT_GOAL := help

BIN ?= release-tools
TMP_DIR ?= .tmp
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || git describe --tags --always --dirty 2>/dev/null || printf '%s' dev)
BIN_PATH := $(TMP_DIR)/$(BIN)
GO_TMP_DIR := $(CURDIR)/$(TMP_DIR)/go-build
GO_CACHE_DIR := $(CURDIR)/$(TMP_DIR)/go-cache
GO_ENV := CGO_ENABLED=0 GOTMPDIR=$(GO_TMP_DIR) GOCACHE=$(GO_CACHE_DIR)
LD_FLAGS := -X main.releaseToolsVersion=$(VERSION)
RELEASE_TOOLS ?= $(GO_ENV) go run -ldflags "$(LD_FLAGS)" ./cmd/release-tools

.PHONY: help build test verify container-test helm-registry-test helm-oci-signing-test helm-provenance-test codeberg-smoke-test check snapshot clean

## Show available maintainer targets
help:
	@printf '%s\n' 'Available targets:'
	@awk '\
		/^## / { help = substr($$0, 4); next } \
		/^[a-zA-Z0-9_.-]+:/ { \
			if (help != "") { \
				target = $$1; \
				sub(/:.*/, "", target); \
				printf "  %-24s %s\n", target, help; \
				help = ""; \
			} \
		} \
	' $(MAKEFILE_LIST) | sort

## Build the release-tools CLI into .tmp/
build:
	mkdir -p "$(GO_TMP_DIR)" "$(GO_CACHE_DIR)"
	$(GO_ENV) go build -ldflags "$(LD_FLAGS)" -o "$(BIN_PATH)" ./cmd/release-tools

## Run the full local verification suite
test:
	scripts/test

## Run the default verification suite
verify: test

## Run verification in the dev container
container-test:
	scripts/in-container make verify

## Run Podman-backed Helm registry smoke tests
helm-registry-test:
	scripts/test-helm-registries

## Run Podman-backed Helm OCI Cosign smoke test
helm-oci-signing-test:
	scripts/test-helm-oci-signing

## Run disposable GPG-backed Helm provenance smoke test
helm-provenance-test:
	scripts/test-helm-provenance

## Run live Codeberg release and Helm package smoke test
codeberg-smoke-test:
	scripts/test-codeberg-smoke

## Validate GoReleaser configuration
check:
	mkdir -p "$(GO_TMP_DIR)" "$(GO_CACHE_DIR)"
	$(RELEASE_TOOLS) check

## Build a local snapshot release
snapshot:
	mkdir -p "$(GO_TMP_DIR)" "$(GO_CACHE_DIR)"
	$(RELEASE_TOOLS) snapshot

## Remove generated build and test artifacts
clean:
	rm -rf dist "$(TMP_DIR)" "$(BIN)"
