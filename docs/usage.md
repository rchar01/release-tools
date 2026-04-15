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
- `VERSION`

Local maintainer auth should come from:

```bash
export CODEBERG_TOKEN="$(cat ~/.config/codeberg/token)"
```

CI should provide `CODEBERG_TOKEN` through repository secrets.

`CODEBERG_TOKEN` is the only public token variable. The toolkit maps it to
`GITEA_TOKEN` internally only when invoking Goreleaser.
