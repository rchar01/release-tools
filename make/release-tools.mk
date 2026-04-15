# Public configuration variables for consuming repositories:
#   RELEASE_PROJECT       Project name used by release scripts
#   RELEASE_OWNER         Codeberg/Gitea owner
#   RELEASE_REPO          Repository name (defaults to RELEASE_PROJECT)
#   RELEASE_NOTES_SOURCE  Path to release notes file
#   GORELEASER_CONFIG     Path to GoReleaser config
#   VERSION               Tag/version for release-tag
#
# Advanced/internal overrides:
#   RELEASE_API_URL
#   RELEASE_DOWNLOAD_URL
#   RELEASE_NOTES_MODE
#   RELEASE_BODY_MODE

RELEASE_TOOLS_DIR ?= .tmp/release-tools/current
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

.PHONY: release-tools-check release-guard release-tag-guard \
		release-check release-snapshot release release-tag release-notes

## Check release tools
release-tools-check:
	$(RELEASE_TOOLS_BIN)/ensure-tools.sh

## Check required vars
release-guard:
	@test -n "$(strip $(RELEASE_PROJECT))" || { echo "RELEASE_PROJECT is required"; exit 1; }
	@test -n "$(strip $(RELEASE_OWNER))" || { echo "RELEASE_OWNER is required"; exit 1; }

## Check VERSION
release-tag-guard:
	@test -n "$(strip $(VERSION))" || { echo "VERSION is required, e.g. VERSION=v1.2.3"; exit 1; }

## Check release config
release-check: release-tools-check release-guard
	$(RELEASE_TOOLS_BIN)/release-check.sh

## Build snapshot
release-snapshot: release-tools-check release-guard
	$(RELEASE_TOOLS_BIN)/release-snapshot.sh

## Publish current tag
release: release-tools-check release-guard
	$(RELEASE_TOOLS_BIN)/release-current.sh

## Publish one tag
release-tag: release-tools-check release-guard release-tag-guard
	$(RELEASE_TOOLS_BIN)/release-tag.sh

## Generate notes
release-notes: release-tools-check release-guard
	$(RELEASE_TOOLS_BIN)/release-notes.sh
