# Release Tools

This repository is the shared release toolkit consumed by repositories through
the `tools/release` submodule path.

The contract is:

- keep repo-specific artifact configuration in the consuming repo
- keep shared release behavior in `bin/`
- call the shared entrypoints from `make` and `just`

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

Local maintainer auth should come from:

```bash
export GITEA_TOKEN="$(cat ~/.config/codeberg/token)"
```

CI should provide `GITEA_TOKEN` through repository secrets.
