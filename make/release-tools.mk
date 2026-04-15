# Public configuration variables for consuming repos:
#   RELEASE_PROJECT       project name used by release scripts
#   RELEASE_OWNER         codeberg/gitea owner
#   RELEASE_REPO          repository name (defaults to RELEASE_PROJECT)
#   RELEASE_NOTES_SOURCE  path to release notes file
#   GORELEASER_CONFIG     path to goreleaser config
#   VERSION               tag/version for release-tag target
#
# Advanced/internal overrides:
#   RELEASE_API_URL
#   RELEASE_DOWNLOAD_URL
#   RELEASE_NOTES_MODE
#   RELEASE_BODY_MODE

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
VERSION ?=

RELEASE_TOOLS_BIN := $(RELEASE_TOOLS_DIR)/bin

export RELEASE_PROJECT RELEASE_OWNER RELEASE_REPO \
	   RELEASE_API_URL RELEASE_DOWNLOAD_URL \
	   RELEASE_NOTES_SOURCE RELEASE_NOTES_MODE RELEASE_BODY_MODE \
	   GORELEASER_CONFIG VERSION

.PHONY: release-tools-check release-check release-snapshot release release-tag release-notes

release-tools-check:
	$(RELEASE_TOOLS_BIN)/ensure-tools.sh

release-check: release-tools-check ## Validate Goreleaser configuration
	$(RELEASE_TOOLS_BIN)/release-check.sh

release-snapshot: release-tools-check ## Build local release artifacts without publishing
	$(RELEASE_TOOLS_BIN)/release-snapshot.sh

release: release-tools-check ## Publish a tagged release from the current checkout
	$(RELEASE_TOOLS_BIN)/release-current.sh

release-tag: release-tools-check ## Publish a specific tag from an isolated clone (set VERSION=vX.Y.Z)
	$(RELEASE_TOOLS_BIN)/release-tag.sh

release-notes: release-tools-check ## Generate release notes for VERSION or current tag
	$(RELEASE_TOOLS_BIN)/release-notes.sh
