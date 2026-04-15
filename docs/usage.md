# Release Tools

This repository is the shared release toolkit consumed by repositories through
a pinned runtime checkout.

The contract is:

- keep repo-specific artifact configuration in the consuming repo
- keep shared release behavior in `bin/`
- call the shared entrypoints from `make`
- run shared scripts from the bootstrapped toolkit while targeting the consumer repo through `RELEASE_REPO_ROOT`

Recommended consumer pattern:

- pin a toolkit tag in a repo-local version file such as `.release-tools-version`
- bootstrap that exact tag into `.tmp/release-tools/<version>`
- update `.tmp/release-tools/current` to the active checkout
- set repo-specific variables in the consumer `Makefile`
- include `.tmp/release-tools/current/make/release-tools.mk`

`release-tag` still publishes from a clean clone of the exact repo tag, but it
uses the current bootstrapped toolkit scripts against that clone by setting
`RELEASE_REPO_ROOT`.

Expected consumer variables:

- `RELEASE_PROJECT`
- `RELEASE_OWNER`
- `RELEASE_REPO`
- `RELEASE_API_URL`
- `RELEASE_DOWNLOAD_URL`
- `RELEASE_NOTES_SOURCE`
- `RELEASE_NOTES_MODE`
- `RELEASE_BODY_MODE`
- `GORELEASER_CONFIG`
- `VERSION`

Required for release commands:

- `RELEASE_PROJECT`
- `RELEASE_OWNER`

Additionally required for `make release-tag`:

- `VERSION`

Local maintainer auth should come from:

```bash
export CODEBERG_TOKEN="$(cat ~/.config/codeberg/token)"
```

CI should provide `CODEBERG_TOKEN` through repository secrets.

`CODEBERG_TOKEN` is the only public token variable. The toolkit maps it to
`GITEA_TOKEN` internally only when invoking Goreleaser.

The shared Make frontend fails fast when required variables are missing:

- `make release-check`
- `make release-snapshot`
- `make release`
- `make release-notes`
  require `RELEASE_PROJECT` and `RELEASE_OWNER`
- `make release-tag`
  requires `RELEASE_PROJECT`, `RELEASE_OWNER`, and `VERSION`

Generated release notes are release body text only. The toolkit does not add a
top-level Markdown heading for the tag because the forge already renders the
release title separately.
