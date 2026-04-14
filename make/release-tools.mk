RELEASE_TOOLS_DIR ?= tools/release
RELEASE_PROJECT ?=
RELEASE_OWNER ?=
RELEASE_REPO ?= $(RELEASE_PROJECT)
RELEASE_API_URL ?= https://codeberg.org/api/v1
RELEASE_DOWNLOAD_URL ?= https://codeberg.org
RELEASE_NOTES_SOURCE ?= NEWS.md
RELEASE_NOTES_MODE ?= news-md
RELEASE_BODY_MODE ?= none
GORELEASER_CONFIG ?= .goreleaser.yaml

RELEASE_TOOLS_BIN := $(RELEASE_TOOLS_DIR)/bin

export RELEASE_PROJECT
export RELEASE_OWNER
export RELEASE_REPO
export RELEASE_API_URL
export RELEASE_DOWNLOAD_URL
export RELEASE_NOTES_SOURCE
export RELEASE_NOTES_MODE
export RELEASE_BODY_MODE
export GORELEASER_CONFIG

.PHONY: release-check release-snapshot release release-tag release-notes

release-check: ## Validate Goreleaser configuration
	$(RELEASE_TOOLS_BIN)/release-check.sh

release-snapshot: ## Build local release artifacts without publishing
	$(RELEASE_TOOLS_BIN)/release-snapshot.sh

release: ## Publish a tagged release from the current checkout
	$(RELEASE_TOOLS_BIN)/release-current.sh

release-tag: ## Publish a specific tag from an isolated clone (set VERSION=vX.Y.Z)
	$(RELEASE_TOOLS_BIN)/release-tag.sh

release-notes: ## Generate release notes for VERSION or current tag
	$(RELEASE_TOOLS_BIN)/release-notes.sh
